---
title: Example Document
menuTitle: Example Document
weight: 30
description: An example document to use when creating new documents
---

All documents will have a header line that Hugo uses to render the document and the navigation menu. The header should look like this:

```yaml
---
title: My Supercool Document
menuTitle: Supercool Doc
weight: 10
description: The coolest document you'll never read because it doesn't really exist.
---
```

The **title** field will be the title that appears at the top of the document. The **menuTitle** field will be used in the menu structure if the doc is a root document (`_index.md`) of a section. The **weight** is an integer that determines the document's placement in the menu order. Lower weights will be at the top, with the next higher weight following after it. The **description** field should be a short description of the document. The description will show up when a list of child documents is generated on a parent page.

After a short introduction explaining the purpose of this particular document, we move into the first section.

## Section 1

The first section should use the `##` for a heading size. The `#` heading size is a bit too big.

Start each main section with an explanation about what will be in the section. If this is a step-by-step guide, then summarize what they did in the previous section and what they are about to do.

### Section 1.1

Make frequent use of sub-headings. Readers are going to skim. The heading is their flag to slow down and see if the content is relevant to what they are looking for.

#### Section 1.1.1

This is probably as deep as any sub-heading should go. If you're tempted to use a `#####` sub-heading, that's a good clue that you should break the document up into multiple docs.

The end of a major section should have a horizontal rule to help the reader understand that this section has come to a close.  The horizontal rule is placed by using three dashes `---`.

---

## Section 2

Code examples can be inline by using a pair of back-ticks: `` `code goes here` ``. You can also add a code block by using three back-ticks and specifying the language being used. For instance to put a block of yaml, you would do this:

````
```yaml
software: gloo
version: 1.2
```
````

You can refer to other sections of the documentation by using a shortcode for a version link from root. This will refer to the document from the root of the docs site with versioning taken into consideration. For example, a link to this document would look like this:

```
[Example Doc]({{</* versioned_link_path fromRoot="/contributing/example_doc/" */>}})
```

You can also use shortcode to add a **note** blurb to your document. For example the following shortcode:

```
{{%/* notice note */%}}
Here's something you should be aware of. You're awesome!
{{%/* /notice */%}}
```

would be rendered like this:

{{% notice note %}}
Here's something you should be aware of. You're awesome!
{{% /notice %}}

---

## Next Steps

Most documents should end with a summary of what the person just read, and what they should do next. Should they read another document? Is there a video they could watch? Is there a tutorial to run through or a Slack channel dedicated to this topic? The point is to help give the reader some direction on what they might want to do with all this newfound knowledge.
