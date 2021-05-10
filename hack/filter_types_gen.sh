#!/usr/bin/env bash

# File arguments are linked by position in each corresponding array
filepaths=("../projects/envoyinit/cmd/utils/filter_types.gen.go" "../projects/gloo/cli/pkg/xdsinspection/filter_types.gen.go")
packages=("utils" "xdsinspection")
include_gloo=(false true)

for idx in {0..1}
do
  echo "// Copyright Istio Authors" > ${filepaths[$idx]}
  echo "//" >> ${filepaths[$idx]}
  echo "// Licensed under the Apache License, Version 2.0 (the \"License\");" >> ${filepaths[$idx]}
  echo "// you may not use this file except in compliance with the License." >> ${filepaths[$idx]}
  echo "// You may obtain a copy of the License at" >> ${filepaths[$idx]}
  echo "//" >> ${filepaths[$idx]}
  echo "//     http://www.apache.org/licenses/LICENSE-2.0" >> ${filepaths[$idx]}
  echo "//" >> ${filepaths[$idx]}
  echo "// Unless required by applicable law or agreed to in writing, software" >> ${filepaths[$idx]}
  echo "// distributed under the License is distributed on an \"AS IS\" BASIS," >> ${filepaths[$idx]}
  echo "// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied." >> ${filepaths[$idx]}
  echo "// See the License for the specific language governing permissions and" >> ${filepaths[$idx]}
  echo "// limitations under the License.\n" >> ${filepaths[$idx]}
  echo "//  GENERATED FILE -- DO NOT EDIT\n" >> ${filepaths[$idx]}
  echo "package ${packages[$idx]}\n\nimport (" >> ${filepaths[$idx]}
  go list github.com/envoyproxy/go-control-plane/... | grep "v[2-9]" | xargs -n1 -I{} sh -c 'echo "\t_ \"{}\""' >> ${filepaths[$idx]}

  if ${include_gloo[$idx]}
  then
    echo "\n// gloo filter types" >> ${filepaths[$idx]}
    go list github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/... | xargs -n1 -I{} sh -c 'echo "\t_ \"{}\""' >> ${filepaths[$idx]}
  fi

  echo ")" >> ${filepaths[$idx]}
done