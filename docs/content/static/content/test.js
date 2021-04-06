const MINOR_RELEASE = "minorrelease"
const CHRONOLOGICAL = "chronological"
const COMPARE_VERSIONS = "compareversions"

const HASH_SEPARATOR = "_"
function copyAnchorToClipboard(elem){
  str = window.location.href.split('#')[0] + '#' +elem.id
  const el = document.createElement('textarea');
  el.value = str;
  el.setAttribute('readonly', '');
  el.style.position = 'absolute';
  el.style.left = '-9999px';
  document.body.appendChild(el);
  el.select();
  document.execCommand('copy');
  document.body.removeChild(el);
  // e.preventDefault();
}
// Extension for adding header anchor links
showdown.extension('header-anchors', function() {

	var ancTpl = '$1<a id="user-content-$3" onclick="copyAnchorToClipboard($3)" class="anchor" href="#$3" aria-hidden="true"><svg aria-hidden="true" class="octicon octicon-link" height="16" version="1.1" viewBox="0 0 16 16" width="16"><path fill-rule="evenodd" d="M4 9h1v1H4c-1.5 0-3-1.69-3-3.5S2.55 3 4 3h4c1.45 0 3 1.69 3 3.5 0 1.41-.91 2.72-2 3.25V8.59c.58-.45 1-1.27 1-2.09C10 5.22 8.98 4 8 4H4c-.98 0-2 1.22-2 2.5S3 9 4 9zm9-3h-1v1h1c1 0 2 1.22 2 2.5S13.98 12 13 12H9c-.98 0-2-1.22-2-2.5 0-.83.42-1.64 1-2.09V6.25c-1.09.53-2 1.84-2 3.25C6 11.31 7.55 13 9 13h4c1.45 0 3-1.69 3-3.5S14.5 6 13 6z"></path></svg></a>$4';

  return [{
    type: 'html',
    regex: /(<h([1-5]) id="([^"]+?)">.*)(<\/h\2>)/g,
    replace: ancTpl
  }];
});

const changelogTypeSelect = $("#select-type");

function evaluateHash() {
  const hash = window.location.hash.substr(1);
  let changelogType = MINOR_RELEASE;
  if (hash.split(HASH_SEPARATOR).length > 0){
    switch (hash.split(HASH_SEPARATOR)[0]){
      case CHRONOLOGICAL:
      case MINOR_RELEASE:
      case COMPARE_VERSIONS:
        changelogType = hash.split(HASH_SEPARATOR)[0];
        break
      }
  }
  changelogTypeSelect.val(changelogType);
  generateChangelog(changelogType);
}
evaluateHash();
window.onhashchange = evaluateHash

document.querySelector("#select-type").addEventListener("change", e => {
  window.location.hash = e.target.value;
});

var changelogJsonData = null

let changelogData = {data: undefined};
const changelogDataSetter = new Proxy(changelogData, {
  set: function(_, key, value){
    if (key === "data"){
      $("#changelogdiv").html(value);
      if (document.querySelector(window.location.hash)){
        document.querySelector(window.location.hash).scrollIntoView();
      }
    }
    return true;
  }
});

function getText(){
  if (!changelogPath){
    console.err("changelogPath must be defined")
  }
  return fetch(changelogPath)
  .then(response => response.json()).then(resJson => resJson);
}   

function generateChangelog(type){
  if (!changelogJsonData){
    getText().then(json => {
      changelogJsonData = json;
      changelogDataSetter.data = generateMarkdown(type, changelogJsonData);
    });
  }else{
    changelogDataSetter.data = generateMarkdown(type, changelogJsonData);
  }
}

function getRenderer(type){
  switch(type){
    case CHRONOLOGICAL:
      return new ChronologicalRenderer();
    case MINOR_RELEASE:
      return new MinorReleaseRenderer();
    case COMPARE_VERSIONS:
      return new VersionComparer();
  }
}

function generateMarkdown(type, input){
  return getRenderer(type).renderMarkdown(new ReleaseData(input));;
}

function H1 (s){
	return `\n# ${s}\n`
}

function H2 (s ) {
	return `\n## ${s}\n`
}

function H3 (s ) {
	return `\n### ${s}\n`
}

function H4 (s ) {
	return `\n#### ${s}\n`
}

function H5 (s ) {
	return `\n##### ${s}\n`
}

function H5 (s ) {
	return `\n###### ${s}\n`
}

function Bold (s ) {
	return `**${s}**`
}

function Italic (s ) {
	return `*${s}*`
}

function OrderedListItem (s )  {
	return `1. ${s}\n`
}

function UnorderedListItem (s)  {
	return `- ${s}\n`
}

function Link(title, link )  {
	return `[${title}](${link})`
}

function Collapsible(title, content){
  return `\n<details><summary>\n${title}</summary>\n${content}</details>\n `
}

class ReleaseData{
  constructor(input){
    if (!input instanceof Array){
      throw new Error(`Expecting array to ReleaseData ctor input`);
    }
    this.versionData = new Map();
    for (let obj of input){
      this.versionData[Object.keys(obj)[0]] = new VersionData(Object.values(obj)[0])
    }
  }
}

class VersionData{
  constructor(input){
    if (!input instanceof Array){
      throw new Error("Expecting array to VersionData ctor input");
    }
    // Map gaurantees order of insertion
    this.changelogNotes = new Map();
    for (let obj of input){
      this.changelogNotes[Object.keys(obj)[0]] = new ChangelogNotes(Object.values(obj)[0])
    }
  }
}

class ChangelogNotes{
  constructor(input){
    this.categories = input.Categories || {}
    this.extraNotes = input.ExtraNotes
    this.headerSuffix = input.HeaderSuffix
    this.createdAt = input.CreatedAt
  }

  add(otherChangelogNotes){
    for (const [header, notes] of Object.entries(otherChangelogNotes.categories)){
      for (const note of notes){
        if (!this.categories[header]){
          this.categories[header] = []
        }
        this.categories[header].push(note);
      }
    }
  }
}

class MarkdownRenderer{
  render(obj){
    if (obj instanceof ChangelogNotes){
      return this.renderChangelogNotes(obj);
    }
    if (obj instanceof VersionData){
      return this.renderVersionData(obj);
    }
    if (obj instanceof ReleaseData){
      return this.renderReleaseData(obj);
    }
  }

  renderMarkdown(input){
    const renderer = new showdown.Converter({
      extensions: this.discludeHeaderAnchors ? []: ['header-anchors'],
      headerLevelStart: 3,
      prefixHeaderId: this.headerIdPrefix+HASH_SEPARATOR
    });
    const markdown = this.render(input);
    return renderer.makeHtml(markdown);
  }
}

class MinorReleaseRenderer extends MarkdownRenderer{
  constructor(){
    super()
    this.headerIdPrefix = MINOR_RELEASE
  }

  renderChangelogNotes(input){
    var output = "";
    for (const [header,notes] of Object.entries(input.categories)){
      output += H5(header);
      for (const note of notes){
        output += UnorderedListItem(note);
      }
    }
    return output
  }

  renderVersionData(input){
    var output = "";
    for (const [header, notes] of Object.entries(input.changelogNotes)){

      output += H3(header + notes.headerSuffix);
      output += this.renderChangelogNotes(notes);
    }
    return output
  }

  renderReleaseData(input){
    var output = "";
    for (const [header, notes] of Object.entries(input.versionData)){
      output += H2(header);
      output += this.renderVersionData(notes);
    }
    return output
  }
}


class ChronologicalRenderer extends MarkdownRenderer{
  constructor(){
    super()
    this.headerIdPrefix = CHRONOLOGICAL
  }
  renderChangelogNotes(input){
    input.sort((a, b) => {return b[1].createdAt - a[1].createdAt});
    let output = ""
    for (const [header, changelogNotes] of input){
      output += H3(header + changelogNotes.headerSuffix)
      for (const [category,notes] of Object.entries(changelogNotes.categories)){
        output += H4(category);
        for (const note of notes){
          output += UnorderedListItem(note);
        }
      }
    }
    return output
  }

  renderVersionData(input){
    var notes = []
    for (const versionData of input){
      for (const [version, data] of Object.entries(versionData.changelogNotes)){
        notes.push([version, data])
      }
    }
    let output = this.renderChangelogNotes(notes)
    return output
  }

  renderReleaseData(input){
    var notes = []
    for (const [_, version] of Object.entries(input.versionData)){
      notes.push(version)
    }
    let output = this.renderVersionData(notes);
    return output
  }
}


let previousVersion_compare = new Proxy({}, {
  set: function(obj, key, value){

    return true;
  }
});
let laterVersion_compare;

class VersionComparer{
  constructor(){
    this.headerIdPrefix = COMPARE_VERSIONS
    this.discludeHeaders = ["Dependency Bumps", "Pre-release"]
  }

  renderVersionData(input){
    var notes = []
    for (const versionData of input){
      for (const [version, data] of Object.entries(versionData.changelogNotes)){
        notes.push([version, data])
      }
    }
    let output = this.renderChangelogNotes(notes)
    return output
  }

  renderChangelogNotes(input){
    var output = "";
    let sortedByCategory = Object.entries(input.categories).sort((a , b) => a[0] > b[0])
    for (const [header,notes] of sortedByCategory){
      if (this.discludeHeaders.includes(header)){
        continue;
      }
      let section = ""
      for (const note of notes){
        section += UnorderedListItem(note);
      }
      output += Collapsible(`${header} (${notes.length})`, section)
    }
    return output
  }

  markdownToHtml(markdown){
    const renderer = new showdown.Converter({
      extensions: this.discludeHeaderAnchors ? []: ['header-anchors'],
      headerLevelStart: 3,
      prefixHeaderId: this.headerIdPrefix+HASH_SEPARATOR
    });
    return renderer.makeHtml(markdown);
  }

  onSelectChange(){
    let index = this.select.prop('selectedIndex');
    let index1 = this.select1.prop('selectedIndex');
    let select1Val = this.select1.val();
    this.select1.empty();
    Object.entries(this.versions).slice(0,index).forEach(([k,v]) => this.select1.append($("<option>").attr('value', k).text(k)))
    this.select1.val(select1Val);
    this.select1.prop("disabled", false);
    let data = new Map()
    for (const [version, changelogNotes] of Object.entries(this.versions).slice(index1, index+1)){
      for (const [header, notes] of Object.entries(changelogNotes.categories).sort((a, b) => a[0] > b[0])){
        if (!this.discludeHeaders.includes(header)){
          for (const note of notes){
            if (!data[header]){
              data[header] = {}
            }
            if (!data[header][version]){
              data[header][version] = []
            }
            data[header][version].push(note);
          }
        }
      }
    }
    let output = "";
    for (const [header, versionData] of Object.entries(data)){
      let noteStr = "";
      let count = 0
      for (const [version, notes] of Object.entries(versionData)){
        noteStr += H4("Added in " + version);
        for (const note of notes){
          noteStr += UnorderedListItem(note);
          count += 1
        }
      }
      output += Collapsible(`${header} (${count})`, noteStr)
      
    }
    $("#compareversionstextdiv").html(this.markdownToHtml(output))
  }

  renderMarkdown(releaseData){
    this.versions = new Map();
    for (const [, v] of Object.entries(releaseData.versionData)){
      for (const [version, data] of Object.entries(v.changelogNotes)){
        // Don't include betas, only include releases
        // if (version.includes('-')){
        //   continue;
        // }
        this.versions[version] = data;
      }
    }
    var parentDiv = $('<div />')
    var textDiv = $('<div id="compareversionstextdiv"/>')
    var div = $('<div style="display:flex;width:20%;justify-content:space-between"/>')
    this.select = $('<select/>')
    this.select.change(this.onSelectChange.bind(this));
    Object.entries(this.versions).forEach(([k]) => this.select.append($("<option>").attr('value', k).text(k)))
    this.select1 = $('<select/>').prop('disabled', true)
    this.select1.change(this.onSelectChange.bind(this));
    Object.entries(this.versions).forEach(([k]) => this.select1.append($("<option>").attr('value', k).text(k)))
    div.append(this.select).append(this.select1)
    parentDiv.append(div).append(textDiv)
    return parentDiv
  }

  

}