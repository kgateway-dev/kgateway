# Documentation

To contribute to the documentation, edit the markdown files in the `docs/` directory. 
Note that the docs are built using Hugo, which has some differences in the site structure 
from GitHub. 

To test docs changes, you must run `make serve-site` and actually test the 
site in the browser. This will rebuild the site when the source files change automatically, 
so once you start serving the site you can simply iterate between the text editor and browser.  

## Linking between files

The markdown syntax for Hugo crosslinks is slightly different to normal markdown, due to 
a few nuances about how the site is generated. 

* All links to other markdown pages should drop the ".md" extension
* When loading a page (i.e. `gloo.solo.io/introduction/architecture`) Hugo actually loads the file 
`gloo.solo.io/introduction/architecture/index.html`. Relative links need to be prefaced with `../` 
when files are in the same directory. 

For example, relative links from `docs/introduction/architecture` to `docs/introduction/concepts` write the link as:
`[concepts](../concepts)`. 

## Front matter

Content must start with front matter, such as:

```

---
title: "Architecture"
weight: 2
---
```

Otherwise it won't be included in the navigation menu. 

Use the weights to set the content ordering. Pages in the same section are ordered by weight, then alphabetical. 