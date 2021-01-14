# Note to developers/testers
# >> to force docs publication and deployment during testing, do the following:
# cd to repo root, connect to a cluster where you want to push your images, run the following (update SUFFIX as you like)
# SUFFIX=a GCLOUD_PROJECT_ID=solo-public TAGGED_VERSION=vtest-docs-build$SUFFIX make publish-docs -B && kubectl apply -f docs/install/manifest-latest.yaml -n docs
# >> to run host the docs locally, do the following
# cd to the docs dir
# make serve-site -B



# remove the "v" prefix
VERSION ?= $(shell echo $(TAGGED_VERSION) | cut -c 2-)

# replace the patch version number with "0"
DOCS_VERSION := $(word 1,$(subst ., ,$(VERSION))).$(word 2,$(subst ., ,$(VERSION))).0

# Passed by cloudbuild
GCLOUD_PROJECT_ID := $(GCLOUD_PROJECT_ID)

GCR_REPO_PREFIX := gcr.io/$(GCLOUD_PROJECT_ID)
IMAGE_REPO := $(GCR_REPO_PREFIX)/gloo-docs

# this allows us to host docs.solo.io/gloo/vX.Y.Z or docs.solo.io/supergloo/vA.B.C
PRODUCT_SCOPE := gloo

HUGO_VERSION := 0.69.2

SOLO_HUGO_THEME_REVISION := 5a29d20f92fabafb2e41ec0576320e284b624c5e

#----------------------------------------------------------------------------------
# Docs
#----------------------------------------------------------------------------------

.PHONY: site-common
site-common: clean
	if [ ! -d themes/hugo-theme-soloio ]; then git clone https://github.com/solo-io/hugo-theme-soloio themes/hugo-theme-soloio; fi
	git -C themes/hugo-theme-soloio checkout $(SOLO_HUGO_THEME_REVISION)
	GO111MODULE=on go run cmd/generate_changelog_doc.go gen-changelog-md gloo > content/static/content/gloo-changelog.docgen
	GO111MODULE=on go run cmd/generate_changelog_doc.go gen-changelog-md glooe > content/static/content/glooe-changelog.docgen

.PHONY: site-test
site-test: site-common
	GO111MODULE=on go run cmd/generate_changelog_doc.go gen-version-scope-data --no-scope
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml
	# ensure that valid json is generated. Common cause: using yaml ">" multiline string symbols in Hugo's toml headers
	cat site/index.json | jq "." > /dev/null

.PHONY: site-release
site-release: site-common
	GO111MODULE=on go run cmd/generate_changelog_doc.go gen-version-scope-data --product gloo --version $(VERSION) --call-latest
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml
	# ensure that valid json is generated. Common cause: using yaml ">" multiline string symbols in Hugo's toml headers
	# (if it passes here, it will pass on the subsequent generation so no need to check both hugo calls)
	cat site/index.json | jq "." > /dev/null
	mv site site-latest
	GO111MODULE=on go run cmd/generate_changelog_doc.go gen-version-scope-data --product gloo --version $(DOCS_VERSION)
	hugo --config docs.toml
	mv site site-versioned

## Old way of deploying site. TODO: remove when old hosting setup is disabled
# .PHONY: deploy-site
# deploy-site: site
# 	firebase deploy --only hosting:gloo-docs

.PHONY: serve-site
serve-site: site-test
	# ensure hugo version locally installed matches the version in cloudbuild CI
	hugo version | grep -q $(HUGO_VERSION)
	hugo --config docs.toml --themesDir themes server -D

.PHONY: clean
clean:
	rm -fr ./site ./resources ./site-latest ./site-versioned ./data/Solo.yaml

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


# use "current" instead of the default "latest" so we don't accidentally push something with this tag 
CURRENT_TAG:=current

.PHONY: docker-push-docs
docker-push-docs: site-release manifest
ifeq ($(ON_DEFAULT_BRANCH), true)
	docker build \
		--build-arg VERSION=latest \
		--build-arg PRODUCT_SCOPE=$(PRODUCT_SCOPE) \
		--build-arg FROM_DIR=./site-latest \
		-t $(GCR_REPO_PREFIX)/gloo-docs:$(CURRENT_TAG) .
	docker push $(GCR_REPO_PREFIX)/gloo-docs:$(CURRENT_TAG)
endif
ifeq ($(RELEASE),true)
	docker build \
		--build-arg VERSION=$(DOCS_VERSION) \
		--build-arg PRODUCT_SCOPE=$(PRODUCT_SCOPE) \
		--build-arg FROM_DIR=./site-versioned \
		-t $(GCR_REPO_PREFIX)/gloo-docs:$(VERSION) .
	docker push $(GCR_REPO_PREFIX)/gloo-docs:$(VERSION)
endif

.PHONY: manifest
manifest:
	helm template install/helm \
		--set docsVersion=$(DOCS_VERSION) \
		--set image.tag=$(VERSION) \
		--set image.repository=$(IMAGE_REPO) \
		> install/manifest-versioned.yaml
	helm template install/helm \
		--set docsVersion=latest \
		--set image.tag=$(CURRENT_TAG) \
		--set image.repository=$(IMAGE_REPO) \
		> install/manifest-latest.yaml
