# Documentation

Edit pages in `content/` and navigation in its `_meta.js` files.
The site uses Nextra 4; theme settings live in `app/layout.jsx`.

Use Node 24 and run commands from the repository root.

## Local development

```sh
./Taskfile docs npm ci
./Taskfile docs dev
```

## Static build

```sh
TARGET=static BASE_PATH=/indexit ./Taskfile docs build
```

Output: `docs/dist/`. Omit `BASE_PATH` when hosting at the domain root.
The [Pages workflow](../.github/workflows/cd.docs.yml) supplies this path automatically.

`package.json` pins Zod to `4.1.12` for Nextra and its theme to avoid
[a validation regression](https://github.com/shuding/nextra/issues/5036).
Revisit these overrides when upgrading Nextra.
