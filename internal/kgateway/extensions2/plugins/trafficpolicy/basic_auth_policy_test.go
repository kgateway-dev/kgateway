package trafficpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAndFilterSHAUsers(t *testing.T) {
	tests := []struct {
		name            string
		htpasswdData    string
		expectedValid   []string
		expectedInvalid []string
	}{
		{
			name:            "empty htpasswd data",
			htpasswdData:    "",
			expectedValid:   []string{},
			expectedInvalid: []string{},
		},
		{
			name:            "single valid SHA user",
			htpasswdData:    "user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
			expectedValid:   []string{"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{},
		},
		{
			name: "multiple valid SHA users",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=
user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
				"user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=",
			},
			expectedInvalid: []string{},
		},
		{
			name:            "MD5 hash is filtered out",
			htpasswdData:    "alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.",
			expectedValid:   []string{},
			expectedInvalid: []string{"alice"},
		},
		{
			name:            "bcrypt hash is filtered out",
			htpasswdData:    "bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2",
			expectedValid:   []string{},
			expectedInvalid: []string{"bob"},
		},
		{
			name: "mixed valid and invalid users",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=
bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2
user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
				"user3:{SHA}d95o2uzYI7q7tY7bHI4U1xBug7s=",
			},
			expectedInvalid: []string{"alice", "bob"},
		},
		{
			name: "empty lines and comments are skipped",
			htpasswdData: `# Comment line
user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=

# Another comment
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=

`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{},
		},
		{
			name: "whitespace is trimmed",
			htpasswdData: `  user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=  
	user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=	`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{},
		},
		{
			name:            "malformed entry without colon",
			htpasswdData:    "invalidentry",
			expectedValid:   []string{},
			expectedInvalid: []string{"invalidentry"},
		},
		{
			name: "malformed entries mixed with valid ones",
			htpasswdData: `user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
malformedentry
user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=`,
			expectedValid: []string{
				"user1:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
				"user2:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=",
			},
			expectedInvalid: []string{"malformedentry"},
		},
		{
			name:            "username with special characters",
			htpasswdData:    "user@example.com:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=",
			expectedValid:   []string{"user@example.com:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{},
		},
		{
			name:            "password with colon in hash (multiple colons in line)",
			htpasswdData:    "user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=:extra",
			expectedValid:   []string{},
			expectedInvalid: []string{"user"},
		},
		{
			name:            "crypt hash is filtered out",
			htpasswdData:    "user:rl5FQ9fW/7E6A",
			expectedValid:   []string{},
			expectedInvalid: []string{"user"},
		},
		{
			name: "all users invalid - only MD5 and bcrypt",
			htpasswdData: `alice:$apr1$3zSE0Abt$IuETi4l5yO87MuOrbSE4V.
bob:$2y$05$r3J4d3VepzFkedkd/q1vI.pBYIpSqjfN0qOARV3ScUHysatnS0cL2`,
			expectedValid:   []string{},
			expectedInvalid: []string{"alice", "bob"},
		},
		{
			name: "duplicate valid users",
			htpasswdData: `user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs=
user:{SHA}2kuSN7rMzfGcB2DKt67EqDWQELA=`,
			expectedValid:   []string{"user:{SHA}NWoZK3kTsExUV00Ywo1G5jlUKKs="},
			expectedInvalid: []string{"user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validUsers, invalidUsernames := validateAndFilterSHAUsers(tt.htpasswdData)
			assert.Equal(t, tt.expectedValid, validUsers, "valid users mismatch")
			assert.Equal(t, tt.expectedInvalid, invalidUsernames, "invalid usernames mismatch")
		})
	}
}
