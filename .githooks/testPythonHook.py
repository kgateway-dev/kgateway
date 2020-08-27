#!/usr/bin/python3

import subprocess

def main():
    print("Hello World!")
    #if git diff HEAD~1 | grep '^+' | grep -Eq "[A-Za-z0-9_=-]{20,}\.[A-Za-z0-9_=-]{20,}" ; then
    lines = subprocess.check_output(['git', 'diff', 'HEAD~1']).decode("utf-8").split('\n')
    lines2 = list(filter(lambda line : len(line) > 10 and line[0] == '+', lines))
    for line in lines2:
        print(line)

if __name__ == "__main__":
    main()
