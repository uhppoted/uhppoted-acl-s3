module github.com/uhppoted/uhppoted-app-s3

go 1.14

require (
	github.com/aws/aws-sdk-go v1.34.5
	github.com/uhppoted/uhppote-core v0.6.4
	github.com/uhppoted/uhppoted-api v0.6.4
	golang.org/x/sys v0.0.0-20200812155832-6a926be9bd1d
)

replace (
	github.com/uhppoted/uhppote-core => ../uhppote-core
	github.com/uhppoted/uhppoted-api => ../uhppoted-api
)
