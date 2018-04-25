# How to manually create the circleci secrets

Arrange secrets like so:

```
secrets/
secrets/aws/
secrets/aws/config
secrets/aws/credentials
secrets/kube/
secrets/kube/config
```

tar with

```
tar cvf secrets.tar secrets
```

encrypt with

```
docker run -ti -v ${PWD}:/foo soloio/circleci \
    openssl aes-256-cbc -e \
    -in /foo/secrets.tar \
    -out /foo/secrets.tar.enc \
    -k ${ENCRYPTION_KEY}
```
