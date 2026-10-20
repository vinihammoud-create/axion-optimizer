module axion

go 1.24.0

toolchain go1.24.7

require (
	github.com/go-ole/go-ole v1.3.0
	github.com/jchv/go-webview2 v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.36.0
)

replace github.com/jchv/go-webview2 => ./third_party/go-webview2

replace golang.org/x/sys => ./third_party/sys

replace github.com/go-ole/go-ole => ./third_party/go-ole
