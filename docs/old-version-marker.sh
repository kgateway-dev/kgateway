# Run from inside the container to add the previous version warning in the header
# and add a robots.txt file to disable search engine crawling

# Submit version number to script, e.g. $ old-version-marker.sh 1.4.0
VERSION=$1

# Move to web files location
cd /usr/share/nginx/html/no_auth/gloo/$VERSION/

# Find all html files and look for the <div class=burger> line that is part of the standard header
# and add the warning message
find . -name "*.html" | tr '\n' '\0' | xargs -0 -n 1 sed -i '' -e 's#<div class="burger">#<div><mark>You are on a previous version of Gloo. The most current docs can be found <a href="https://docs.solo.io/gloo/latest/">here</a>.</mark></div><div class="burger">#'

# Add the robots.txt file to the root of the web site
cat <<EOF >> robots.txt
User-agent: *
Disallow: /
EOF