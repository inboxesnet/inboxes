# Spec: HaveIBeenPwned Password Breach Check

## Problem

Users can set passwords that appear in known data breaches, as long as they meet complexity rules (8+ chars, upper + lower + digit). Breached passwords are the #1 cause of account compromise.

## Solution

Integrate the [Pwned Passwords API](https://haveibeenpwned.com/API/v3#PwnedPasswords) to check passwords against 850M+ known breached passwords.

## API Details

- **Cost:** Free, no API key required
- **Privacy:** Uses k-anonymity — only first 5 chars of SHA-1 hash sent, server returns all matching suffixes, check happens locally
- **Endpoint:** `GET https://api.pwnedpasswords.com/range/{first5HashChars}`
- **Response:** Newline-separated list of hash suffixes with occurrence counts
- **Rate limit:** None documented for Pwned Passwords (separate from breach API)

## Implementation

### Backend (`handler/helpers.go`)

Add to `validatePassword()`:

```go
func isPasswordPwned(password string) (bool, error) {
    hash := sha1.Sum([]byte(password))
    hashStr := strings.ToUpper(fmt.Sprintf("%x", hash))
    prefix := hashStr[:5]
    suffix := hashStr[5:]

    resp, err := http.Get("https://api.pwnedpasswords.com/range/" + prefix)
    if err != nil {
        return false, err // fail open — don't block signup if API is down
    }
    defer resp.Body.Close()

    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := scanner.Text()
        parts := strings.SplitN(line, ":", 2)
        if strings.EqualFold(parts[0], suffix) {
            return true, nil // password found in breach database
        }
    }
    return false, nil
}
```

### Endpoints to Check
- `POST /api/auth/signup` — on initial password set
- `POST /api/auth/reset-password` — on password reset
- `POST /api/auth/claim` — on invite claim (sets password)
- `PATCH /api/users/me/password` — on password change

### Error Handling
- **API down:** Fail open (allow the password). Don't block signups because HIBP is unreachable.
- **Breached password:** Return 400 with message: "This password has appeared in a data breach. Please choose a different password."
- **Timeout:** 3-second timeout on HTTP call. Fail open on timeout.

### Frontend
- Add hint text below password fields: "Password must not appear in known data breaches"
- Show specific error message when backend rejects breached password

## Considerations
- SHA-1 is used only for the HIBP lookup (not for storage — bcrypt still used for that)
- No full password ever leaves our server
- ~500ms typical response time from HIBP API
- Consider caching common prefixes in Redis (TTL 24h) to reduce external calls

## Priority
Low-medium. Good security hygiene, easy to implement, free.
