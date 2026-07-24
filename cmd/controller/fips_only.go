//go:build fips140v1.0

// The fips140v1.0 build tag is set automatically when building with
// GOFIPS140=v1.0.0 (the skaffold "fips" profile), so this only applies to the
// controller-fips image. It upgrades the runtime default from fips140=on to
// fips140=only: any use of a non-FIPS-approved algorithm panics instead of
// silently succeeding.

//go:debug fips140=only

// Disable the X25519MLKEM768 hybrid key share: its X25519 component is not
// FIPS-approved, and Go generates the default key share before fips140=only
// filtering, so TLS handshakes panic without this. With it off, the client
// leads with P-256.
//go:debug tlsmlkem=0

package main
