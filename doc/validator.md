# Validation

Registration defaults define conversion types. Set, SetSource and complete source
loads reject incompatible values before publication. Overflow, fractional integer
conversion, non-finite values and invalid textual duration/time/network values error.
Untyped nil defaults leave the value type open within the supported value graph.

`Validate(paths...)` requires a registered path to have a supplied value, even if it
equals the default. `RegisterRequired` and `required:"true"` fields are collected by
`Validate()` with no arguments and by Build. Defaults alone do not satisfy required
inputs. Registration of a struct is staged, so a duplicate/invalid field or invalid
explicit environment value cannot leave a partially registered schema.

Build supports `WithValidator(func(*Config) error)` and a matching
`WithTypedValidator(func(*T) error)`. They run at initialization. They are not rerun
on runtime changes: apply application-level constraints before adopting a new
snapshot when those constraints must hold after reconfiguration.

Helpers include Port, Positive, NonNegative, IPAddress, IPv4Address, IPv6Address,
URLPath, OneOf, Range, Pattern and NonEmpty. Pattern compiles its expression when the
validator is created and panics on an invalid expression. Network validators allow
the documented empty/unspecified defaults. Use errors.Is with ErrValidation,
ErrTypeMismatch and ErrDecode to handle failures.
