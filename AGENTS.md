## Testing

Run make test to run all tests.

Integration and e2e tests require the development environment mode:

    ANANSI_ENV=development make test

Production mode (default) requires UUIDv7 field IDs; without the dev env, schema
fixtures that use plain field IDs fail with `ERR_REGISTRY_INVALID_SCHEMA`
("Field ID '...' is not a valid UUIDv7").

## Bug fixing

Should you discover a bug while working on the codebase, fixing the bug takes precedence over whatever else you are doing.
