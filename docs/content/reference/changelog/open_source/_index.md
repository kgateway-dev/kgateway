---
title: Open Source Gloo Edge
weight: 7
description: Changelogs for Open Source Gloo Edge
---

<br>

<script>
const render = function (template) {
    let htmlToSet;
    const node = document.querySelector("#changelogdiv");
    if (!node) return;
    if (template === "chronological"){
    htmlToSet = `{{< readfile file="static/content/gloo-changelog.docgen" markdown="true" >}}`;
    }
    else if (template === "minor-release"){
        htmlToSet = `{{< readfile file="static/content/gloo-minor-release-changelog.docgen" markdown="true" >}}`;
    }
    node.innerHTML = htmlToSet;
}
</script>

## Changelog
<select name="type" id="select-type" onchange="javascript:render(this.value);">
    <option value="minor-release">By Release</option>
    <option value="chronological">By Chronological Order</option>
</select>
<div id="changelogdiv">
{{< readfile file="static/content/gloo-minor-release-changelog.docgen" markdown="true" >}}
</div>
