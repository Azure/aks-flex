---
name: update-aksflexnode
description: Updates the AKSFlexNode dependency to a specified version across all Go modules, vendor directories, and source constants in the aks-flex repository.
---

# Update AKSFlexNode

This skill updates the `github.com/Azure/AKSFlexNode` dependency to a target version across the entire aks-flex repository.

## When to Use This Skill

Use this skill when the user:

- Asks to update, bump, or upgrade AKSFlexNode to a new version
- Mentions a new AKSFlexNode release that needs to be adopted
- Asks to change the flex node version

## Locations to Update

There are several places where the AKSFlexNode version is referenced. All must be updated together to keep the repository consistent.

### 1. Source constant — `flexNodeVersion`

**File:** `plugin/pkg/services/agentpools/userdata/flex/flex.go`

Update the `flexNodeVersion` constant to the new version string. This constant is used at runtime to download the correct AKSFlexNode release tarball via the bootstrap template.

```go
const (
	flexNodeVersion = "<new-version>"
)
```

### 2. Go module — `plugin/go.mod`

This is the direct dependency. Update the version in the `require` block:

```
github.com/Azure/AKSFlexNode <new-version>
```

### 3. Go module — `karpenter/go.mod`

This is an indirect dependency. Update the version in the indirect `require` block:

```
github.com/Azure/AKSFlexNode <new-version> // indirect
```

### 4. Go module — `cli/go.mod`

This is an indirect dependency (via a `replace` directive pointing to `../plugin`). Update the version in the indirect `require` block:

```
github.com/Azure/AKSFlexNode <new-version> // indirect
```

### 5. Checksum files — `go.sum`

Do **not** edit `go.sum` files manually. They are updated automatically in the next step.

## Post-Update Steps

After editing the files above, run the following commands **in order**:

### Step 1: `go mod tidy` in `plugin/`

The `plugin` module is the direct consumer of AKSFlexNode and must be tidied first since other modules depend on it.

```bash
cd plugin && go mod tidy
```

### Step 2: `go mod tidy` in `karpenter/` and `cli/`

These can be run in parallel:

```bash
cd karpenter && go mod tidy
cd cli && go mod tidy
```

### Step 3: `make vendor-patch` in `karpenter/`

The `karpenter` module uses a vendor directory. After tidying, run the vendor-patch target to re-vendor and apply any necessary patches:

```bash
cd karpenter && make vendor-patch
```

### Step 4: Verify

Confirm no stale version references remain:

```bash
grep -r "AKSFlexNode" --include="*.go" --include="go.mod" --include="go.sum" plugin/ karpenter/ cli/ | grep -v vendor/
```

All lines should reference the new version. The vendored files under `karpenter/vendor/` will also reflect the update from the `go mod vendor` step.

## Notes

- The bootstrap template (`plugin/pkg/services/agentpools/userdata/flex/assets/bootstrap.sh.tmpl`) uses `{{ .Version }}` which is populated from the `flexNodeVersion` constant at runtime — it does not need to be edited directly.
- The `README.md` and `docs/` mention AKSFlexNode by name but do not contain version strings, so they do not need updating.
