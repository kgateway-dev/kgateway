# Custom Linting with Ruleguard

This folder contains custom rules we define ourselves for this particular project. If you find an anti-pattern that 
you want to prevent, 
you should first check to see if there's a linter in the list [here](https://golangci-lint.run/usage/linters/) to 
make sure there is not a check that already exists. However, if you can't find such a check, it is fairly easy to 
build a custom check using the ruleguard DSL.

## Writing Rules

Rather than getting into details of how to write rules, here are some links to the most useful documentation:

* [Ruleguard DSL documentation](https://github.com/quasilyte/go-ruleguard/blob/master/_docs/dsl.md)
* [Ruleguard by example](https://go-ruleguard.github.io/by-example)
* [Ruleguard Project README](https://github.com/quasilyte/go-ruleguard#readme)
* [Ruleguard Project Documentation Links](https://github.com/quasilyte/go-ruleguard#documentation) 

## Directory Layout

This directory must be flat. Subdirectories of this directory will not be checked for Golang files. This is 
controlled by the `/linters-settings/gocritic/settings/ruleguard/rules` setting in `.golangci.yaml`. Often, rules 
are all piled into a single file, but I've tried to break them up a little bit to keep a single file of rules from 
being difficult to manage. Break them as best you see fit.