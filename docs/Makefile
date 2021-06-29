# Note to developers/testers
# >> to run host the docs locally, do the following
# cd to the docs dir
# make serve-site -B



# remove the "v" prefix
VERSION ?= $(shell echo $(TAGGED_VERSION) | cut -c 2-)

HUGO_VERSION := 0.69.2

SOLO_HUGO_THEME_REVISION := 41587dc9dc719bd22cedd9803994f756e25f571f

#----------------------------------------------------------------------------------
# Docs
#----------------------------------------------------------------------------------

.PHONY: site-common
site-common: clean gloo-enterprise-version
	if [ ! -d themes/hugo-theme-soloio ]; then git clone https://github.com/solo-io/hugo-theme-soloio themes/hugo-theme-soloio; fi
	git -C themes/hugo-theme-soloio checkout $(SOLO_HUGO_THEME_REVISION)
	GO111MODULE=on go run cmd/generate_docs.go gen-releases gloo > opensource.out
	GO111MODULE=on go run cmd/generate_docs.go gen-releases glooe > enterprise.out
	GO111MODULE=on go run cmd/generate_docs.go gen-changelog-md gloo > content/static/content/gloo-changelog.docgen
	GO111MODULE=on go run cmd/generate_docs.go gen-changelog-md glooe > content/static/content/glooe-changelog.docgen
	GO111MODULE=on go run cmd/generate_docs.go gen-security-scan-md gloo > content/static/content/gloo-security-scan.docgen
	GO111MODULE=on go run cmd/generate_docs.go gen-security-scan-md glooe > content/static/content/glooe-security-scan.docgen
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

# Generate _output/gloo-enterprise-version.
# NOTE: file paths in hack/find_latest_enterprise_version.go assume the current working 
# directory is the root of the project, so we need to navigate up one level first.
.PHONY: gloo-enterprise-version
gloo-enterprise-version:
	cd .. && GO111MODULE=on go run hack/find_latest_enterprise_version.go