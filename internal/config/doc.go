// Package config will own strict JSON v1 decoding and schema validation.
// It must reject unknown/duplicate keys and trailing data before any connection.
// It is independent of CLI and transport implementations.
package config
