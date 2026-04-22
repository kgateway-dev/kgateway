# Adding a New Envoy Filter to the Dynamic Module

All filters are compiled into the single `librust_module.so` loaded by Envoy.
Each filter is a separate Rust crate under `filters/` and registered in the
`module-init` cdylib entry point.

See `filters/kgateway-example-filter/` for a minimal working skeleton to copy from.

## Steps

### 1. Create the filter crate

Copy the example skeleton and rename it:

```bash
cp -r filters/kgateway-example-filter filters/<your-filter-name>
```

Edit `filters/<your-filter-name>/Cargo.toml`:
- Set `name = "<your-filter-name>-filter"`
- For logic that might be shared among other filters, create a library crate under lib/ as dependencies

Edit `filters/<your-filter>/src/lib.rs`:
- Define `pub struct FilterConfig` and implement `HttpFilterConfig<EHF>`
- Define `pub struct PerRouteConfig` (remove if you don't need per-route overrides)
- Define a private `struct Filter` implementing `HttpFilter<EHF>`
- Do **not** call `declare_init_functions!` — that lives only in `module-init`

### 2. Register in the workspace

**File: `Cargo.toml`** (workspace root — `internal/envoy_modules/Cargo.toml`)

Add your crate to the `members` list:

```toml
members = [
    ...
    "filters/<your-filter-name>",   # <-- add this line
    ...
]
```

### 3. Add as a dependency of the cdylib

**File: `module-init/Cargo.toml`**

```toml
[dependencies]
...
<your-filter-name>-filter = { path = "../filters/<your-filter-name>" }
```

### 4. Register the filter name in the dispatch functions

**File: `module-init/src/lib.rs`**

In `new_http_filter_config_fn`, add a `match` arm:

```rust
"<your-filter-name>" => <your_filter_name>_filter::FilterConfig::new(filter_config)
    .map(|c| Box::new(c) as Box<dyn HttpFilterConfig<EHF>>),
```

In `new_http_filter_per_route_config_fn`, add the corresponding arm:

```rust
"<your-filter-name>" => <your_filter_name>_filter::PerRouteConfig::new(per_route_config)
    .map(|c| Box::new(c) as Box<dyn Any>),
```

Also update the panic message in each function to include `<your-filter-name>` in the
known filters list.

### 5. Verify

```bash
cd internal/envoy_modules
make lint
make test
```

### 6. Build 

This will rebuild the envoy-wrapper image that will include the envoy binary, envoyinit and the dynamic module:
(run at repo-root)

```bash
make envoy-wrapper-docker
```
