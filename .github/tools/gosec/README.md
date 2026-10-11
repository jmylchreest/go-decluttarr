# gosec build dependencies

gosec 2.29.0 bundles an older golang.org/x/tools that cannot decode Go 1.27
export data. This isolated module pins the same scanner with x/tools 0.52.0
and its resolved dependencies. CI builds the scanner here, then scans the
application from the repository root. Runtime dependencies are unaffected.
Renovate's gomod manager can maintain this module.

Remove the override/module when a released gosec supports the active compiler.
