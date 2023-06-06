---
title: Contributing to the Gloo Edge Documentation
menuTitle: Documentation
weight: 20
description: Read this guide to help make Gloo Edge docs the best they can be!
---

Hi! So you'd like to help us out with the Gloo Edge documentation. That is awesome! Before you get started, we have a couple things to mention about contributing standards and process. We'd like to make this easy as possible, so if you have a tiny edit to make, check out the [Quickstart](#quickstart) section of the guide. Otherwise, here are the ways you can get involved:

* Log documentation issues
* Correct some minor errors on a page
* Revise documents and create new ones

---

## Background

The Gloo Edge docs live in the `docs/content` directory of the Gloo Edge GitHub repository. The docs are written in Markdown and rendered to a static site using [Hugo](https://gohugo.io/). The docs make use of *shortcodes* from Hugo and some custom shortcodes that are part of a solo-io theme stored in [this repository](https://github.com/solo-io/hugo-theme-soloio). Shortcodes are a way to render custom HTML on a page without injecting that HTML directly into Markdown.

The docs website can be rendered locally for updates and testing by using Hugo and `make`. There are some software prerequisites that need to be fulfilled in order to render and view the site successfully. More information can be found in the **[Install prerequisite software]({{< versioned_link_path fromRoot="/contributing/documentation/editing_locally/#install-prerequisite-software" >}})** section of the Editing Locally guide.

For minor changes and edits, it is not necessary to clone the repository and render it locally. You can simply make the changes directly on GitHub and submit a pull request (PR).

---

## Log an issue

The simplest way to get involved with docs is by submitting an issue when you see something that needs to be changed. It could be a grammatical error, an unclear statement, or a suggestion for a new document or example. If you see something that should be amended or improved, head on over to the Gloo Edge GitHub repository and follow the [steps in the Quickstart](#log-an-issue-on-the-main-gloo-edge-repository) for logging an issue.

---

## Quickstart

While it is possible to clone the entire Gloo Edge repository, make changes to the documentation, render it locally to validate, and then submit a PR; that's a lot of work. For minor edits and small improvements, it is much simpler to edit the docs directly on GitHub and submit a pull request. Here is the quickstart process:

1. Log an issue on the main Gloo Edge repository 
2. Fork the Gloo Edge repository to your own account
3. Make the update and commit it
4. Create a pull request to merge the change

Once the PR has been submitted, someone from solo.io will review the change and either approve it or ask for more information. See below for an example of creating and submitting a change.

### Log an issue on the main Gloo Edge repository  

In this step you will log an issue on the Gloo Edge GitHub repository so that others know you are working on a fix. You can also simply log an issue with the documentation to let others know about the problem.

* On the Gloo Edge repository, go to the **Issues** tab and click on the **New Issue** button.
* Click on **Get started** for a Bug report.
* Add the label **Area: Docs** to the Bug report
* Fill out the form with the issue you found in the docs and what changes you plan to make
* Click on **Submit new issue**

### Fork the Gloo Edge repository to your own account

In this step you will fork the Gloo Edge repository into your own account. This step assumes that you already have a GitHub account. More information on forking a repository can be found on [GitHub's website](https://guides.github.com/activities/forking/).

* On the Gloo Edge repository, click on the **Fork** button at the top of the screen
* Select your account as a destination for the fork

After a few moments the fork will complete and you will be taken to the page with your fork of the Gloo Edge repository. Now you can make edits and submit a PR.

### Make the update and commit it

In this step you will make the actual change to the document that has an issue. From the forked repository, select the branch you want to make changes to. The current branch being used to generate the Gloo Edge docs is `main`. The files that make up the docs website are in `docs/content`, and the directory structure follows the menu structure of the docs site.

* Select the correct branch to edit (`main`)
* Find the file you want to edit and click the **pencil** icon
* Make your changes to the file
* Select the **Create a new branch** option
* Name the branch something descriptive, e.g. hello-world-grammar-fix
* Click on **Commit new file**

The change has been committed to a new branch on your forked repository.

### Create a pull request to merge the change

Now that the change has been committed to your fork of the Gloo Edge repository, it's time to submit a pull request to merge the change into the official Gloo Edge repository.

* Go back to the main page of your forked repository and click on **New pull request**
* Select the proper branch for the Gloo Edge repository (`main`) and the branch you just created
* Click on **Create pull request**
* Fill out the comment field with the changes made and reference the Issue created earlier
* Select the label **Area: Docs**
* Click on **Create pull request**
* Celebrate! You're awesome for helping.

Once the PR is submitted, someone from solo.io will review the change and either approve it or ask for more information.

---

## Making big updates

While making minor edits and fixes works well on GitHub directly, more involved changes require using a proper editor and rendering the site locally. If you'd like to contribute at that level, we recommend reading through our [style guide]({{< versioned_link_path fromRoot="/contributing/documentation/style_guide/" >}}) and setting up your [local system properly]({{< versioned_link_path fromRoot="/contributing/documentation/editing_locally/" >}}).

## Next Steps

- Check out the [style guide]({{< versioned_link_path fromRoot="/contributing/documentation/style_guide/" >}})
- Find [existing open](https://github.com/solo-io/gloo/labels/Area%3A%20Docs) issues
- Participate in the [community Slack](https://slack.solo.io/)!

