---
title: Style Guide
menuTitle: Style Guide
weight: 20
description: Standards and guidance for Gloo Edge documentation
---

The documentation for Gloo Edge is meant to be conversational and engaging. It should also be easy to read and inviting for the newcomer. In an attempt to maintain a consistent voice and style throughout the docs, please use the following conventions when approaching a new document or revising an existing one.

## Style guide

* **Introduce and guide** - All documents should start with a title and introduction. Assume that the reader ended up on this page without reading any other documents. Don't assume that they are coming to this document with deep technical knowledge about Gloo Edge. Be supportive and guide the reader to be successful.

* **Italicize terms** - When a technical term is first used in a document, it should be *italicized* to indicate to the reader that the term has a technical meaning. This is true for any technical term that is not commonly used by the general public. You might know what an *ingress controller* is, but don't assume your audience does. In addition, the term should be explained when it is first mentioned, either within the paragraph that contains it, or in [] square brackets immediately after the term.

* **Bold UI elements** - When referring to a specific UI element such as a **button** or **form field**, the reference should be in **bold**. This clues the reader in on the fact that you're talking about the **OK** button and not just telling them that they're OK.

* **Code formatting for code and files** - When a snippet of code appears in your document, or you refer to a filename or directory path, those items should use the `in-line code` format. Code snippets that are more than a few words long should be placed in a code block instead.

* **Expand acronyms on first use** - When a document has an acronym, even if it is a common one, expand it out when it is first used. For instance, *AWS* should be expanded to *Amazon Web Services (AWS)* the first time it is used on a page. For less common acronyms, it may make sense to expand its first use in each major section of the document to avoid confusion for readers who may be skimming.

* **Use shorter paragraphs** - No single paragraph should be longer than four or five sentences. Each paragraph should encapsulate a new concept or idea. Single-line paragraphs can be a powerful tool to highlight a key concept or thought.

* **Short, clear sentences** - Sentences that run on for multiple lines or have more than one comma should be broken up into multiple sentences.

* **Use everyday words** - While you may have memorized the thesaurus, now is not the time to prove it. Vocabulary should be accessible to an average adult reader. Keep things accurate and technical, but not obtuse or obscure.

* **Use subheadings** - Split the document up into as many sub-sections as makes sense. Readers will often skim a document for information that is relevant to them. Each heading serves as a marker for the reader to check-in before moving to the next section. Make sure the title of each heading is descriptive and succinct.

* **Multiple documents are okay** - We're not killing trees here, and creating a new document is not that difficult. If you find you've got more than three major sections or you are delving into a fourth-level for subheadings, then it's probably time to split the document into multiple docs.

* **Conversational style** - Your docs and edits should feel like a conversation between you - the author - and the person reading the doc. We're not writing academic papers for a scholarly journal, but we are also not writing a text message on a Friday night. Be personable and professional. If you're unsure, just ask!

* **Use Title Case for titles** - The title of each document should in title case. If you're not sure about proper formatting when it comes to title case, then check out this [website](https://titlecaseconverter.com/).

* **Use Sentence Case for headings** - Headings for sections should be in Sentence Case. Sentence Case simply means that the first word should be capitalized, along with any proper nouns. All other words should be in lower case.

* **Single space after a period** - This might be confusing to those old and young alike. The short version is that the docs for Gloo Edge use a single space after a period. Period.

---

## Document naming and structure

The docs use Hugo to generate the static website, which makes the structure and naming of the directories holding the content important. The structure of the current docs can be found on the [Gloo Edge GitHub repository](https://github.com/solo-io/gloo). Here is a representation of `content` directory as of this writing.

```bash
# Menu item in the navigation bar
├───api
├───changelog
├───cli
├───contributing
├───dev
├───getting_started
├───gloo_integrations
├───gloo_routing
│   ├───hello_world # Nested menu item
│   ├───tls
│   ├───validation
│   ├───virtual_services
│   └───_index.md # Base page of the gloo_routing menu item
├───img
├───installation
├───introduction
├───observability
├───security
├───static
├───upgrading
└───_index.md # Base page of the website

```

Each directory appears as a menu item on the navigation bar. The page that loads when the menu item is clicked will be the `_index.md` file in that directory. Additional files in that directory will appear based on the value assigned to the `title` field in the header of the document. The order of the files is controlled by the value assigned to the `weight` field in the header of the document.

To create a new menu item, simply create a directory at the root of the content directory or in the sub-directory where you want the menu item to appear. Then add an `_index.md` file to that directory and populate it with content. The `weight` value in the `_index.md` file determines the order in which the menu item will appear among the parent items. For instance, the `_index.md` file found in the `introduction` directory has a `weight` value of `10`. This places the **Introduction** menu item at the top of the navigation bar. The `weight` value for additional files in a directory determine their order within the expanded menu item in the navigation pane. More information can be found on the [Hugo website](https://gohugo.io/templates/lists/) if you really want to do a deep dive.

## Next Steps

Now that you have a firm grasp of how the docs should be written, you can start writing your first doc or get set up on your local workstation.

* The [Example Document doc]({{< versioned_link_path fromRoot="/contributing/documentation/example_doc/" >}}) provides a starting point for creating new docs in Gloo Edge
* The [Editing Locally doc]({{< versioned_link_path fromRoot="/contributing/documentation/editing_locally/" >}}) provides a guide for getting started with Gloo Edge docs on your local workstation