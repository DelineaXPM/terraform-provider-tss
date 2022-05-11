module github.com/thycotic/terraform-provider-tss

require (
	github.com/hashicorp/go-getter v1.5.11 // indirect
	github.com/hashicorp/terraform v0.12.14
	github.com/thycotic/tss-sdk-go v1.0.0
)

// replace github.com/thycotic/tss-sdk-go => ../tss-sdk-go

go 1.13
