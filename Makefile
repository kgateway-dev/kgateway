# Deprecated Makefile - Delegates to Taskfile.yml via `go tool task`
#
# See ./README.md regarding task and Taskfile.yml.

TASK ?= go tool task

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "make is deprecated in favor of go tool task; see README.md"
	@$(TASK) $@

.PHONY: %
%:
	@$(TASK) $@
