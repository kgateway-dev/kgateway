#!/usr/bin/python3

# This script runs whenever a user tries to commit something in this repo.
# It checks the commit for any text that resembled an encoded JSON web token,
# and asks the user to verify that they want to commit a JWT if it finds any.
import sys
import subprocess
import re
import base64
import binascii
import unittest

# run test like so:
# (cd .githooks/; python -m unittest pre-commit-python.py)
class TestStringMethods(unittest.TestCase):

    def test_jwts(self):
        self.assertTrue(contains_jwt(["eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.POstGetfAytaZS82wHcjoTyoqhMyxXiWdR7Nn7A29DNSl0EiXLdwJ6xC6AfgZWF1bOsS_TuYI3OG85AmiExREkrS6tDfTQ2B3WXlrr-wp5AokiRbz3_oB4OxG-W9KcEEbDRcZc0nH3L7LzYptiy1PtAylQGxHTWZXtGz4ht0bAecBgmpdgXMguEIcoqPJ1n3pIWk_dUZegpqx0Lka21H6XxUTxiy8OcaarA8zdnPUnV6AmNP3ecFawIFYdvJB_cm-GvpCSbr8G8y_Mllj8f4x9nBH8pQux89_6gUY618iYv7tuPWBFfEbLxtF2pZS6YC1aSfLQxeNe8djT9YjpvRZA"]))
        self.assertTrue(contains_jwt(["eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"]))

    def test_ok(self):
        self.assertFalse(contains_jwt(["test test"]))
        self.assertFalse(contains_jwt(["thisisnotajwteventhoughitisalongstring"]))
 

def contains_jwt(lines):
    jwtPattern = re.compile('JWT|iat|name|sub|alg|exp|k')
    raiseIssue = False
    for line in lines:
        # try to find long (20+ character) words consisting only of valid JWT characters
        longTokens = re.findall("[A-Za-z0-9_=-]{20,}", line)
        # try to decode any found tokens and see if they look like a JSONfragment
        # where :look like a JSON fragment" is defined as "contains any of the words in the 'jwtPattern' regex pattern"
        for token in longTokens:
            try:
                # python's base64 decoder fails if padding is missing; but does not fail if there's
                # extra padding; so always add padding
                utfOut = base64.urlsafe_b64decode(token+'==').decode("utf-8")
                match = jwtPattern.search(utfOut)
                if match:
                    print("Probable JWT found in commit: " + token + " gets decoded into: " + utfOut)
                    raiseIssue = True
            # be very specific about the exceptions we ignore:
            except (UnicodeDecodeError, binascii.Error) as e:
                continue
    return raiseIssue

def main():
    #get git diff lines
    lines = subprocess.check_output(['git', 'diff', '--staged']).decode("utf-8").split('\n')

    # filter out short lines and lines that don't begin with a '+' to only
    # test longer, newly added text
    filteredLines = list(filter(lambda line : len(line) > 20 and line[0] == '+', lines))

    # found a likely JWT, send user through prompt sequence to double check
    if contains_jwt(filteredLines):
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
