# Test Profiles

## What is a Profile?
A profile represents a personality that is installing Gloo Gateway. With a complex Helm API, it can become challenging to understand the various values that should be used. We aggregate these recommendations into profiles to make it easier to categorize.

## How should these be used?
It is important to recognize that profiles contain a set of recommended values for an installation. _However, as with all configuration, these should be tested and fine-tuned before rolling them out in your environment._

We recommend that these profiles are used initially only in tests and easy demos.

## How can this evolve?
Ideally, profiles become a concept that is built directly into the product. In that case, these would be available in our CLI and user-facing documentation.