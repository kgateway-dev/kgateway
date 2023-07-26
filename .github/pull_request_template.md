# Description

Please include a high level summary of the changes.

This bug fixes ... \ This new feature can be used to ...

## API changes
- Added x field to y resource
- ...

## Code changes
- Fix error in `Foo()` function
- Add `Bar()` function
- ...

## CI changes
- Adjusted schedule for nightly jobs
- ...

## Docs changes
- Added guide about feature x to public docs
- Updated README to account for y behavior
- ...

# Context

Users ran into this bug doing ... \ Users needed this feature to ...

See slack conversation [here](https://solo-io-corp.slack.com/archives/some/post)

## Interesting decisions
 
We chose to do things this way because ...

## Testing steps

I manually verified behavior by ...

## Reviewer notes

Be sure to verify intended behavior by ...

Please proofread my comments

This is a complex PR and may require a huddle to discuss ...

# Checklist:

- [ ] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] I have added tests that prove my fix is effective or that my feature works

# Author reminders (delete before opening)
- Include a concise, user-facing changelog (for details, see https://github.com/solo-io/go-utils/tree/main/changelogutils) referencing the issue that is resolved
- Run codegen via `make -B install-go-tools generated-code`
- Follow guidelines laid out in the Gloo Edge [contribution guide](https://docs.solo.io/gloo-edge/latest/contributing/)
- If not ready for review open a draft PR or apply the work in progress label