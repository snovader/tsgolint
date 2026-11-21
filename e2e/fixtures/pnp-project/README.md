# PnP archive fixture

This fixture uses Yarn's external `.pnp.data.json` format. The `.pnp.cjs`
placeholder marks the installation root; the native compiler reads the data
directly, so tests do not require Yarn or execute a PnP loader.

`packages.zip` contains:

- `@fixture/config`: a package exporting `./base` as `./base.json`, with
  `target: ESNext`, `module: NodeNext`, `strict: true`, and `types: []`.
- `@fixture/api`: a package exporting `.` and `./subpath` with declarations for
  `rootPromise(): Promise<void>` and `subpathPromise(): Promise<void>`.

All entries are under `node_modules/` inside the archive. Both package-based
config inheritance and root/subpath type resolution must read the archive.
