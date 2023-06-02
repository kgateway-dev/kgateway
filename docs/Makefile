# Note to developers/testers
# >> to run host the docs locally, do the following
# cd to the docs dir
# make serve-site -B

# Additional steps are required for darwin/arm64 (M1 macOS)
# build hugo v0.69.2:
# https://github.com/gohugoio/hugo must be built from commit ec9dcf304849ded11cea70ab9ad46e0a0a4615b2 and installed on $PATH



VERSION 	?= 1.0.1-dev    # a basic version that can be overidden

HUGO_VERSION := 0.69.2

SOLO_HUGO_THEME_REVISION := v0.0.25

# The minimum version to maintain in our public docs
MIN_SCANNED_VERSION ?= v1.11.0

#----------------------------------------------------------------------------------
# Docs
#----------------------------------------------------------------------------------


.PHONY: site-common
site-common: clean gloo-enterprise-version
	if [ ! -d themes/hugo-theme-soloio ]; then git clone https://github.com/solo-io/hugo-theme-soloio themes/hugo-theme-soloio; fi
	git -C themes/hugo-theme-soloio checkout $(SOLO_HUGO_THEME_REVISION)
	GO111MODULE=on go run cmd/generate_docs.go gen-releases -r gloo > opensource.out
	GO111MODULE=on go run cmd/generate_docs.go gen-releases -r glooe > enterprise.out
	GO111MODULE=on go run cmd/generate_docs.go gen-changelog-md -r gloo > content/static/content/gloo-changelog.docgen
	GO111MODULE=on go run cmd/generate_docs.go gen-changelog-md -r glooe > content/static/content/glooe-changelog.docgen

	# generate split security scans for gloo versions which HAVE received new split-templating updates
	MIN_SCANNED_VERSION=$(MIN_SCANNED_VERSION) GO111MODULE=on go run cmd/generate_docs.go gen-security-scan-md -r gloo > content/static/content/gloo-security-scan.docgen
	MIN_SCANNED_VERSION=$(MIN_SCANNED_VERSION) GO111MODULE=on go run cmd/generate_docs.go gen-security-scan-md -r glooe > content/static/content/glooe-security-scan.docgen
	cat content/static/content/gloo-security-scan.docgen
	./split-file-by-delimiter.sh content/static/content/gloo-security-scan.docgen \\\*\\\*\\\*
	./split-file-by-delimiter.sh content/static/content/glooe-security-scan.docgen \\\*\\\*\\\*

	# get enterprise helm values based on contents of _output/gloo-enterprise-version
	GO111MODULE=on go run cmd/generate_docs.go get-enterprise-helm-values > content/static/content/glooe-values.docgen
	rm -f opensource.out enterprise.out


.PHONY: site-test
site-test: site-common
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml
	# ensure that valid json is generated. Common cause: using yaml ">" multiline string symbols in Hugo's toml headers
	cat site/index.json | jq "." > /dev/null

.PHONY: site-release
site-release: site-common
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml
	# ensure that valid json is generated. Common cause: using yaml ">" multiline string symbols in Hugo's toml headers
	# (if it passes here, it will pass on the subsequent generation so no need to check both hugo calls)
	cat site/index.json | jq "." > /dev/null
	mv site site-latest
	hugo --config docs.toml

.PHONY: serve-site
serve-site: site-test
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml --themesDir themes server -D

.PHONY: clean
clean:
	rm -fr ./site ./resources ./site-latest ./site-versioned ./public ./ci ./gloo-temp ./themes

# Uses https://github.com/gjtorikian/html-proofer
# Does not require running site; just make sure you generate the site and then run it
# Install with gem install html-proofer
# Another option we could use is wget: https://www.digitalocean.com/community/tutorials/how-to-find-broken-links-on-your-website-using-wget-on-debian-7
.PHONY: check-links
check-links:
	htmlproofer ./site/ --empty-alt-ignore  --allow-hash-href --alt-ignore "/img/Gloo-01.png" --url-ignore "/localhost/,/github.com/solo-io/solo-projects/,/developers.google.com/,/getgrav.org/,/github.com/solo-io/gloo/projects/,/developer.mozilla.org/"

# If on fedora, run
#     sudo dnf -y install gcc ruby-devel rubygems zlib-devel
# to install html-proofer deps (only works with gcc, not clang!)
install-tools:
	gem install html-proofer

.PHONY: build-site
build-site: clean
	./build-docs.sh

# Generate _output/gloo-enterprise-version based on the latest patch enterprise version
# corresponding to content in changelog directory.
.PHONY: gloo-enterprise-version
gloo-enterprise-version:
	cd .. && GO111MODULE=on go run hack/find_latest_enterprise_version/main.go

#----------------------------------------------------------------------------------
# Printing makefile variables utility
#----------------------------------------------------------------------------------

# use `make print-MAKEFILE_VAR` to print the value of MAKEFILE_VAR

print-%  : ; @echo $($*)
