# Pull Request Reviews

This doc explains the best practices for reviewing a pull request in the [Gloo Edge project](https://github.com/solo-io/gloo).
If you are looking to contribute to the project, check out the [writing pull requests guide](pull-requests.md).

- [Reviewing Pull Requests](#reviewing-pull-requests)
- [Submitting Reviews](#submitting-reviews)
  - [Ask questions](#ask-questions)
  - [Clarify Nits](#clarify-nits)
  - [Requesting Changes](#requesting-changes)
  - [Providing Guidance](#providing-guidance)
  - [Checking out the code](#checking-out-the-code)
  - [Run tests](#run-tests)
- [Approving Pull Requests](#approving-pull-requests)

## Reviewing Pull Requests
- Read the PR description first, hopefully the author has given important context and hints that will make the diff easier to parse and preemptively answer questions
- Look at tests to see what guarantees are being made. If you can't get a gist of the PR from the tests, consider requesting more
- Verify the semantics of variable names by looking at how they are assigned/used. If it doesn't make sense, consider that it could be because of a flaw in the logic and get clarification.

## Submitting Reviews
### Ask questions
You are encouraged to ask questions and seek an understanding of what the PR is doing. If something isn't clear to you, it is probably best that the PR author clarify it.

When asking questions, try to be mindful with your phrasing. Instead of:

_"Why did you do this?"_

try

_"Am I understanding this correctly? Can you explain why...?"_

### Clarify Nits
Use the prefix “nit” when making a comment about something nitpicky, such as code style/formatting, variable names, etc. Nits express your opinion that something could be better, but that it’s not critical or necessary for correctness or system health. The author can choose to ignore such feedback, but it’s still useful to provide it.

### Requesting Changes
Be mindful of the manner (especially the language) in which you deliver critical feedback, especially feedback that may require the author to make significant changes. This is another occasion where it may be a good idea to talk with the author offline.

That being said, don’t be afraid to give critical feedback (whether that be technical feedback on a PR or interpersonal feedback about behavior). We should all assume good intent and remember that we are on the same team.

### Providing Guidance
If requesting a change, don’t merely say “I don’t like how this is done, please do it differently.” Provide guidance on how it should be done instead.

This can include links to a GitHub issue, a Slack conversation, a design doc, or another project's codebase.

### Checking out the code
Sometimes diffs in the GitHub UI are not enough to understand the changes. You are encouraged to pull down the code locally to better evaluate them:
```
git fetch origin pull/<PR ID>/head:<BRANCHNAME>
git checkout <BRANCHNAME>
```

### Run tests
After pulling down the code, you should try running the tests locally to ensure you understand the changes and that they work as expected.

## Approving Pull Requests
By approving a pull request, you are indicating that you have reviewed the changes and are confident that they are correct and to your understanding they will not cause any issues. You are signing on as a co-author of the changes, and we expect that if you are comfortable approving a PR, you are also comfortable with this responsibility.

If the changes look good to you, but you recognize that you are not the best person to review the changes, instead of approving the PR, you should comment that the changes look good to you, but clarify why you aren't comfortable approving.
