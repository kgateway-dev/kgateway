#!/usr/bin/python3
"""
Analysis script geared towards hunting and removing unwanted dependencies from a project.

Prior runs have yielded results resembling:
```
    PACKAGE="github.com/solo-io/gloo/projects/gloo/cli/cmd"
    TARGET_BASE="github.com/onsi/ginkgo/v2"
    REFRESH_GO_LIST=True
        [github.com/solo-io/gloo/projects/gloo/cli/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/federation/register, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install, github.com/solo-io/solo-kit/test/setup]
        [github.com/solo-io/gloo/projects/gloo/cli/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/debug, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install, github.com/solo-io/solo-kit/test/setup]
        [github.com/solo-io/gloo/projects/gloo/cli/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install, github.com/solo-io/solo-kit/test/setup]
        [github.com/solo-io/gloo/projects/gloo/cli/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check-crds, github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install, github.com/solo-io/solo-kit/test/setup]
```

which were used to remove a dependency on ginkgo by avoiding a solo-kit import
"""

import subprocess
import json
import pickle

PACKAGE         = "github.com/solo-io/gloo/projects/gloo/cli/cmd"   # go module to analyze
TARGET_BASE     = "github.com/onsi/gomega"                          # string match for dependency to hunt
REFRESH_GO_LIST = False                                             # recompute "go list" operation, overwriting saved pickle files

# simple access to bash shell
def shell(cmd):
    process = subprocess.Popen(cmd.split(), stdout=subprocess.PIPE)
    output, _ = process.communicate()
    return output


# run "go list -json {package}" for all dependencies of package and collect data
def run_go_lists(package):
    dependency_map, import_map = {}, {}

    go_list = json.loads(shell(f"go list -json {package}"))

    dependency_map[package] = go_list["Deps"]
    import_map[package] = go_list["Imports"]

    for i, dep in enumerate(go_list["Deps"]):
        go_list = json.loads(shell(f"go list -json {dep}"))
        
        if "Deps" not in go_list:       # some packages have no dependencies
            dependency_map[dep] = []
        else:                           # ...but most _do_ have dependencies
            dependency_map[dep] = go_list["Deps"]

        if "Imports" not in go_list:    # some packages have no imports
            import_map[dep] = []
        else:                           # ...but most _do_ have imports
            import_map[dep] = go_list["Imports"]

        print(f"go list {i}/{len(dependency_map[package])}")
    
    return dependency_map, import_map


# load "go list" data collectors from disk or run them if they don't exist
def load_or_run(package, fresh_run=False):
    if fresh_run:
        dependency_map, import_map = run_go_lists(package)
        pickle.dump(dependency_map, open("dependency_map.p", 'wb'))
        pickle.dump(import_map, open("import_map.p", 'wb'))
        print(f"computed {len(dependency_map)} fresh items")
    else:
        dependency_map = pickle.load(open("dependency_map.p", "rb"))
        import_map = pickle.load(open("import_map.p", "rb"))
        print(f"loaded {len(dependency_map)} items from disk")
    
    return dependency_map, import_map

if __name__ == "__main__":
    # populate "go list" data collectors for all dependencies
    dependency_map, import_map = load_or_run(PACKAGE, REFRESH_GO_LIST)
    
    results = set()
    package_imports = [(
        import_map[PACKAGE], f"[{PACKAGE}"              # to begin with, we consider PACKAGE
        )]
    while len(package_imports) > 0:                     # stop iterating when we are out of packages
        imports, path = package_imports.pop(0)

        imports_target = any([i.startswith(TARGET_BASE) for i in imports])
        if imports_target:                              # if we directly import target: we've found a culprit
            results.add(f"{path}]")
    
        for i in imports:                               # if not, some sub-dependency (or several) are the culprits...
            depends_target = any([d.startswith(TARGET_BASE) for d in dependency_map[i]])
            is_target = i.startswith(TARGET_BASE)

            if depends_target and not is_target:        # ...and we keep recursing
                new_imports = import_map[i]
                new_path = f"{path}, {i}"
                package_imports.append((new_imports, new_path))

    # at present, the results _do not consider_ TestImports or XTestImports.  While I don't know for *sure*
    # I've assumed that go only downloads production dependencies on standard compile operations
    # 
    # If I'm wrong, we can add a similar loop for TestImports/XTestImports, but I'm not sure it's worth the effort
    print(f"{len(results)} matches found for {TARGET_BASE} in {PACKAGE}")
    for r in results:
        print(r)
