<type: fix | feat | build | chore | ci | docs | style | refactor | perf | test >(scope)[!]: [description]

## PR checklist:
- [ ] `docs/` updated
- [ ] tests added
- [ ] an example added to `examples/` (useful to demo new field/resource)
- [ ] `CHANGELOG.md` updated with pending release information

### Example
test(pipeline resource): Add a test to resource_pipeline_test to check for valid name chars

## PR checklist:
- [ ] `docs/` updated <-- No need for a doc update, no user impact
- [x] tests added <-- Tests were added
- [ ] an example added to `examples/` (useful to demo new field/resource) <-- No demo required as no user impact
- [x] `CHANGELOG.md` updated with pending release information <-- Changes were made


#### To ! or not to !
`!` denotes a breaking change, in this example the test does **not** cause a breaking change and so `!` is not required.

A `BREAKING CHANGE` footer may also be used:

```
feat: strip non-ASCII chars from pipeline name
BREAKING CHANGE: no longer replaces non-ASCII with "blank symbols"
```
