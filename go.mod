module github.com/DelineaXPM/terraform-provider-tss/v2

require (
	github.com/DelineaXPM/tss-sdk-go/v2 v2.0.0
	github.com/hashicorp/go-getter v1.6.1 // indirect
	github.com/hashicorp/terraform v1.3.5
)

// replace github.com/DelineaXPM/tss-sdk-go/v2 => ../tss-sdk-go

go 1.13
