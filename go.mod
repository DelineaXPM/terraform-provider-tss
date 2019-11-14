module github.com/thycotic/terraform-provider-tss

require (
	github.com/hashicorp/terraform v0.12.14
	github.com/thycotic/tss-sdk-go v0.0.0-20200116212003-8711a9ea2292
)

replace github.com/thycotic/tss-sdk-go => ../tss-sdk-go

go 1.13
