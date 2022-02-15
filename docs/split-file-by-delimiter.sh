# turn off glob expansion, since default use-case is splitting by lines containing "***""
set -f

# simple parse of input values
file_in=$1      # in expected use-case, is "content/static/content/gloo-security-scan.docgen"
delimiter=$2    # in expected use-case, is "***"

# split file_in to useful components
filename=$(basename -- "$1")
directory_out=$(dirname $1)
base_file_name=`echo "$filename" | cut -d'.' -f1`         # won't work if there are multiple extensions (a.b.js)
file_extension=`echo "$filename" | cut -d'.' -f2`         # won't work if there are multiple extensions (a.b.js)

# do the actual by-line splitting
echo "splitting $1 by $2\n"

i=0
cur_out="$directory_out/$base_file_name-$i.$file_extension"
rm -f $cur_out
cat $file_in | while read line 
do
    if echo "$line" | grep -q "$delimiter"; then
        cur_out="$directory_out/$base_file_name-$i.$file_extension"
        i=$(($i+1))
        rm -f $cur_out
        echo "Generating File: $cur_out"
    fi
    echo $line >> $cur_out
done

echo "\nFinished splitting $file_in into subfiles.  Results are all located in $directory_out."
