#!/usr/bin/python3

# This script runs whenever a user tries to commit something in this repo.
# It checks the commit for any text that resembled an encoded JSON web token,
# and asks the user to verify that they want to commit a JWT if it finds any.
import sys
import subprocess
import re
import base64

def main():
    #get git diff lines
    lines = subprocess.check_output(['git', 'diff', 'HEAD~1']).decode("utf-8").split('\n')
    # filter out short lines and lines that don't begin with a '+' to only
    # test longer, newly added text
    filteredLines = list(filter(lambda line : len(line) > 20 and line[0] == '+', lines))
    
    jwtPattern = re.compile('JWT|iat|name|sub|alg|exp|k')
    raiseIssue = False
    for line in filteredLines:
        # try to find long (20+ character) words consisting only of valid JWT characters
        longTokens = re.findall("[A-Za-z0-9_=-]{20,}", line)
        # try to decode any found tokens and see if they look like a JSONfragment
        # where :look like a JSON fragment" is defined as "contains any of the words in the 'jwtPattern' regex pattern"
        for token in longTokens:
            try:
                #python's base64 decoder is super fragile, and consistently fails to decode JWT words due to unresolvable padding issues
                p1 = subprocess.Popen(["echo", token], stdout=subprocess.PIPE)
                p2 = subprocess.Popen(["base64", "--decode"], stdin=p1.stdout, stdout=subprocess.PIPE)
                p1.stdout.close()  # Allow p1 to receive a SIGPIPE if p2 exits.
                out,err = p2.communicate()
                if err is not None:
                    print("terminal decoding failed while trying to check the following text for JWTs: " + token)
                    continue
                utfOut = out.decode("utf-8")
                
                match = jwtPattern.search(utfOut)
                if match:
                    print("Probable JWT found in commit: " + token + " gets decoded into: " + out.decode("utf-8"))
                    raiseIssue = True
            except Exception as e:
                continue
    # found a likely JWT, send user through prompt sequence to double check
    if raiseIssue:
        prompt = "This commit appears to add a JSON web token, which is often accidental and can be problematic (unless it's for a test). Are you sure you want to commit these changes? (y/n): "
        failCount = 0
        while True:
            inputLine = input(prompt).lower()
            if len(inputLine) > 0 and inputLine[0] == 'y':
                print("OK, proceeding with commit")
                return 0
            elif len(inputLine) > 0 and inputLine[0] == 'n':
                print("Aborting commit")
                return 1
            elif failCount == 0:
                prompt = "Please answer with 'y' or 'n'. Do you wish to proceed with this commit?: "
            elif failCount == 1:
                prompt = "That's still neither a 'y' nor an 'n'. Do you wish to proceed with this commit?: "
            else:
                prompt = "You've entered an incorrect input " + str(failCount) + " times now. Please respond with 'y' or 'n' (sans apostrophes) regarding whether or not you wish to proceed with this commit which possibly contains a JWT: "
            failCount += 1
    else:
        print("No likely JWTs found, proceeding with commit")
        return 0

if __name__ == "__main__":
    sys.exit(main())
