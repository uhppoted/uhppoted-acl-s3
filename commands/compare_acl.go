package commands

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/uhppoted/uhppote-core/device"
	"github.com/uhppoted/uhppote-core/uhppote"
	"github.com/uhppoted/uhppoted-api/acl"
	"github.com/uhppoted/uhppoted-api/config"
	"github.com/uhppoted/uhppoted-api/eventlog"
	"log"
	"net/url"
	"os"
	"strings"
)

var COMPARE_ACL = CompareACL{
	config:      DEFAULT_CONFIG,
	workdir:     DEFAULT_WORKDIR,
	keysdir:     DEFAULT_KEYSDIR,
	credentials: DEFAULT_CREDENTIALS,
	region:      DEFAULT_REGION,
	logFile:     DEFAULT_LOGFILE,
	logFileSize: DEFAULT_LOGFILESIZE,
	noverify:    false,
	nolog:       false,
	debug:       false,
}

type CompareACL struct {
	url         string
	config      string
	workdir     string
	keysdir     string
	credentials string
	region      string
	logFile     string
	logFileSize int
	noverify    bool
	nolog       bool
	debug       bool
}

func (c *CompareACL) Name() string {
	return "compare-acl"
}

func (c *CompareACL) FlagSet() *flag.FlagSet {
	flagset := flag.NewFlagSet("compare-acl", flag.ExitOnError)

	flagset.StringVar(&c.url, "url", c.url, "The S3 URL for the authoritative ACL file")
	flagset.StringVar(&c.credentials, "credentials", c.credentials, "File path for the AWS credentials")
	flagset.StringVar(&c.region, "region", c.region, "The AWS region for S3 (defaults to us-east-1)")
	flagset.StringVar(&c.keysdir, "keys", c.keysdir, "Sets the directory to search for RSA signing keys. Key files are expected to be named '<uname>.pub'")
	flagset.StringVar(&c.config, "config", c.config, "'conf' file to use for controller identification and configuration")
	flagset.StringVar(&c.workdir, "workdir", c.workdir, "Sets the working directory for temporary files, etc")
	flagset.BoolVar(&c.nolog, "no-log", c.nolog, "Writes log messages to stdout rather than a rotatable log file")
	flagset.BoolVar(&c.debug, "debug", c.debug, "Enables debugging information")

	return flagset
}

func (c *CompareACL) Description() string {
	return fmt.Sprintf("Retrieves the ACL from the configured controllers and uploads to S3")
}

func (c *CompareACL) Usage() string {
	return "compare-acl <S3 URL>"
}

func (c *CompareACL) Help() {
	fmt.Println()
	fmt.Printf("  Usage: %s compare-acl <url>\n", SERVICE)
	fmt.Println()
	fmt.Printf("    Retrieves the ACL from the controllers configured in:\n\n")
	fmt.Printf("       %s\n\n", c.config)
	fmt.Printf("    and stores it to the provided S3 URL\n")
	fmt.Println()
	fmt.Println("    Options:")
	fmt.Println()
	fmt.Println("      url         (required) URL for the ACL file. S3 URL's are formatted as s3://<bucket>/<key>")
	fmt.Printf("      credentials (optional) File path for the AWS credentials for use with S3 URL's (defaults to %s)\n", c.credentials)
	fmt.Printf("      region      (optional) AWS region for S3 (defaults to %s)\n", c.region)
	fmt.Printf("      keys        (optional) Directory containing for RSA signing keys (defaults to %s). Key files are expected to be named '<uname>.pub", c.keysdir)
	fmt.Printf("      config      (optional) File path for the 'conf' file containing the controller configuration (defaults to %s)\n", c.config)
	fmt.Println("      no-log      (optional) Disables event logging to the uhppoted-acl-s3.log file (events are logged to stdout instead)")
	fmt.Println("      debug       (optional) Displays verbose debug information")
	fmt.Println()
}

func (c *CompareACL) Execute(ctx context.Context) error {
	if strings.TrimSpace(c.url) == "" {
		return fmt.Errorf("compare-acl requires a URL for the authoritative ACL file in the command options")
	}

	uri, err := url.Parse(c.url)
	if err != nil {
		return fmt.Errorf("Invalid ACL file URL '%s' (%w)", c.url, err)
	}

	conf := config.NewConfig()
	if err := conf.Load(c.config); err != nil {
		return fmt.Errorf("WARN  Could not load configuration (%v)", err)
	}

	u, devices := getDevices(conf, c.debug)

	var logger *log.Logger
	if !c.nolog {
		events := eventlog.Ticker{Filename: c.logFile, MaxSize: c.logFileSize}
		logger = log.New(&events, "", log.Ldate|log.Ltime|log.LUTC)
	} else {
		logger = log.New(os.Stdout, "ACL ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
	}

	return c.execute(&u, uri.String(), devices, logger)
}

func (c *CompareACL) execute(u device.IDevice, uri string, devices []*uhppote.Device, log *log.Logger) error {
	log.Printf("Fetching ACL from %v", uri)

	f := c.fetchHTTP
	if strings.HasPrefix(uri, "s3://") {
		f = c.fetchS3
	}

	b, err := f(uri, log)
	if err != nil {
		return err
	}

	r := bytes.NewReader(b)
	tsv, signature, uname, err := untar(r)

	log.Printf("Extracted ACL from %v: %v bytes, signature: %v bytes", uri, len(tsv), len(signature))

	if !c.noverify {
		if err := verify(uname, tsv, signature, c.keysdir); err != nil {
			return err
		}
	}

	list, err := acl.ParseTSV(bytes.NewReader(tsv), devices)
	if err != nil {
		return err
	}

	for k, l := range list {
		log.Printf("%v  Retrieved %v records", k, len(l))
	}

	current, err := acl.GetACL(u, devices)
	if err != nil {
		return err
	}

	report(current, list, c.workdir, log)

	return fmt.Errorf("OOOPS")
}

func (c *CompareACL) fetchHTTP(url string, log *log.Logger) ([]byte, error) {
	acl, err := fetchHTTP(url, log)
	if err != nil {
		return nil, err
	}

	log.Printf("Fetched ACL from %v (%d bytes)", url, len(acl))

	return acl, nil
}

func (c *CompareACL) fetchS3(url string, log *log.Logger) ([]byte, error) {
	acl, err := fetchS3(url, c.credentials, c.region, log)
	if err != nil {
		return nil, err
	}

	log.Printf("Fetched ACL from %v (%d bytes)", url, len(acl))

	return acl, nil
}
