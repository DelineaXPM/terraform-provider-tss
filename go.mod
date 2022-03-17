module github.com/thycotic/terraform-provider-tss

require (
	github.com/hashicorp/terraform v0.12.14
	github.com/thycotic/tss-sdk-go v1.0.0
	github.com/ulikunitz/xz v0.5.10 // indirect
)

// replace github.com/thycotic/tss-sdk-go => ../tss-sdk-go

go 1.13
