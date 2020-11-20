---
title: Gloo Edge Enterprise
weight: 8
description: Changelogs for Gloo Edge Enterprise
---

<br>
<script>
const render = function (template) {
    let htmlToSet;
    const node = document.querySelector("#changelogdiv");
    if (!node) return;
    if (template === "chronological"){
    htmlToSet = `{{< readfile file="static/content/glooe-changelog.docgen" markdown="true" >}}`;
    }
    else if (template === "minor-release"){
        htmlToSet = `{{< readfile file="static/content/glooe-minor-release-changelog.docgen" markdown="true" >}}`;
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
{{< readfile file="static/content/glooe-minor-release-changelog.docgen" markdown="true" >}}
</div>


