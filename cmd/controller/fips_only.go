//go:build fips140v1.0

// The fips140v1.0 build tag is set automatically when building with
// GOFIPS140=v1.0.0 (the skaffold "fips" profile), so this only applies to the
// controller-fips image. It upgrades the runtime default from fips140=on to
// fips140=only: any use of a non-FIPS-approved algorithm panics instead of
// silently succeeding.

//go:debug fips140=only

package main
