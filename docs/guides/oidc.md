# OIDC Authentication with kgateway

This guide demonstrates how to use kgateway's OAuth2 policy to perform OIDC authentication. The example uses Google as the OIDC provider, but it can be adapted for other providers like Auth0, Keycloak, etc.

## 1. Install kgateway

Follow the installation guide to install kgateway.

## 2. Configure OIDC Provider

Before proceeding, you need to configure an OIDC client application with your provider. For this guide, we use Google.

1.  Follow the guide to [Get your Google API client ID](https://developers.google.com/identity/gsi/web/guides/get-google-api-clientid).
2.  When creating the "Web application" client ID, configure the following:
    *   **Authorized redirect URIs**: Add `https://example.com:8443/oauth2/redirect`
3.  Take note of your **Client ID** and **Client Secret**. You will need them in the next steps.

## 3. Deploy a Backend Application

This guide uses a simple `httpbin` service as the backend application, exposed over HTTPS.

Apply the following manifest to deploy it:

```bash
kubectl apply -f- <<EOF
apiVersion: v1
kind: Service
metadata:
  name: httpbin
spec:
  selector:
    app.kubernetes.io/name: httpbin
  ports:
  - protocol: TCP
    port: 8443
    name: https
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: httpbin
  template:
    metadata:
      labels:
        app.kubernetes.io/name: httpbin
    spec:
      containers:
      - image: docker.io/mccutchen/go-httpbin:2.19.0
        imagePullPolicy: IfNotPresent
        name: httpbin
        command: [go-httpbin]
        args:
        - "-port"
        - "8443"
        - "-https-cert-file"
        - "/etc/certs/tls.crt"
        - "-https-key-file"
        - "/etc/certs/tls.key"
        ports:
        - containerPort: 8443
        volumeMounts:
        - name: certs
          mountPath: /etc/certs
          readOnly: true
      volumes:
      - name: certs
        secret:
          secretName: httpbin-tls
---
# httpbin cert and key generated using:
# openssl req -x509 -out backend.crt -keyout backend.key -newkey rsa:2048 -days 3650 -nodes -sha256 -subj '/CN=backend.example.com' -extensions EXT -config <( printf "[dn]\nCN=backend.example.com\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:backend.example.com\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth")
apiVersion: v1
kind: Secret
metadata:
  name: httpbin-tls
type: kubernetes.io/tls
stringData:
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDDjCCAfagAwIBAgIUDcHvOZe5mvJVNTn3yB+m0K/EQkQwDQYJKoZIhvcNAQEL
    BQAwHjEcMBoGA1UEAwwTYmFja2VuZC5leGFtcGxlLmNvbTAeFw0yNTEyMDEyMjU0
    MzNaFw0zNTExMjkyMjU0MzNaMB4xHDAaBgNVBAMME2JhY2tlbmQuZXhhbXBsZS5j
    b20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDBQ9MdW1HRb0kZZbsx
    DpO4xUmSPIs2M/8qovM+7/jnQndAs/j3hQGIUTGoBI4YsdmsAcHvxue7YnO4s0ZC
    xDRu1TuVAKWpgBGeBezIr0Qb4nszbPqj1+nvNhpWq7Jn8PZD06Kay0mlu82F3LDU
    VL3EePSqJmRvpYt6Y7bY4cZ1MR5Yzyb2pcRR5ueHEG38oVsOgxtJ6tKbS0byL9m/
    3AZVohrK2d9uY/Oij10GWjM7nuY4YKsMVxuZVJnx3oD8kfSFn0eBGs5WT4BZjDCM
    op3xxu7A1eFRrtv+4b06P9IMq9AOjaOYFMU1DZtUyrOToIS/o9ik4DiHR3zjcKGH
    k6UtAgMBAAGjRDBCMB4GA1UdEQQXMBWCE2JhY2tlbmQuZXhhbXBsZS5jb20wCwYD
    VR0PBAQDAgeAMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA0GCSqGSIb3DQEBCwUAA4IB
    AQARwvHK0fYaVeNK7T4h+V+dvi1ZDkCR70WEp8gRB94XiNu7InKnrfzFNDL5nz9Q
    NMu34XNGb6KIwTJTKC1zK+e+oYT9iVgQFvweYtnXmmGPDsJTufWmU90h4/dS2wnC
    tVNm1anyUCy7zw8ofWLKoVmMTZ5cLA0n+3mzIa1xiHv/aybZsGKVh+xFrlVg3xNu
    85jdn3CYv47i0+F2wbMoJbw4/G2pBgq89XWKUI01TZtzaJY2Nwrk3ZS61JY0YGO4
    YD5O7RK1O9jBdp83IwEZs8nohH4yUGfR06yiSFoIoxUP+NfcCdwI6OqvBJYWscZ6
    3hcwvzgG/lOYfUuzkVYOThwr
    -----END CERTIFICATE-----
  tls.key: |
    -----BEGIN PRIVATE KEY-----
    MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQDBQ9MdW1HRb0kZ
    ZbsxDpO4xUmSPIs2M/8qovM+7/jnQndAs/j3hQGIUTGoBI4YsdmsAcHvxue7YnO4
    s0ZCxDRu1TuVAKWpgBGeBezIr0Qb4nszbPqj1+nvNhpWq7Jn8PZD06Kay0mlu82F
    3LDUVL3EePSqJmRvpYt6Y7bY4cZ1MR5Yzyb2pcRR5ueHEG38oVsOgxtJ6tKbS0by
    L9m/3AZVohrK2d9uY/Oij10GWjM7nuY4YKsMVxuZVJnx3oD8kfSFn0eBGs5WT4BZ
    jDCMop3xxu7A1eFRrtv+4b06P9IMq9AOjaOYFMU1DZtUyrOToIS/o9ik4DiHR3zj
    cKGHk6UtAgMBAAECggEBAKJ4GCQXvuJnwXX+Va1Z6cls4Pp0tzsr3xjCv+Zq6j3P
    XF0ibuv1/mHQkAQFQEd2S90T5StjdS/MBiiBXVGHi+SYkWwjjSC/LxA/Pt0+qe0f
    Kh8DQHk4a8rTGrU9xc8nfH9sjMfAmfsftBkSe/0j+BwQ6u2XNNu+uVB8Pxx4QNQG
    rZ2sV4iwk2SR72UaxBqmSuuKJkWoSi40Ttf0B5011ioPJ916P2STO3LFLPzu0I++
    J6s5iWeHFraKNCLaSByZY7r6vDKke125bABNia0TFI91oQnWOd7gozuJTN3MVrKk
    Ch4d49ahc30KKdqFvYWAGSZT0z/pqFcyj2vic6GFgAECgYEA8Sp6sOs3514JWVMT
    gocrhPlODw2vP9AjULIvSmZG3JNpr/nQBE0cMAzoRobJXhL0nDeyouZ3Ci/fvOPt
    h/DlIAZBI8cN+ScSsmawFgHHt7MEdEPdKRh++glZG0EpicuBcNxhlkwjI7+jWSTN
    /cbbzqlMEaQ8wq8VIOhrnYL1lS0CgYEAzScSduUTWMxAarK8vpptRRmsGpzew01E
    ngdDWi1/jM5hCciBvl0baq0rv+KV9wCLB4pfJe2AGI9leudrRFgpGOmT4HACCxWM
    JFIurufW4uzK7td0IoEyn6a5j0BZ3pOx8AoI4JprRNl8Z25y3SHb2S0r+JxhEYeX
    sIcwjB4AUAECgYEA7FaF0BVjPrDwFoKMjxEqO/EZZzUw9idiRHWqVI3wib9JBnSZ
    P23V3tz3UA5NDo0i/Gi0/mE+bVRHPdRcdilEUWLvuUEcV3vMHdr2W0q5TzP3fHz5
    Ionn/d7lXQk5zNkLa+/9Do5krWbjjLu9xyJ3TIqqimtaRCvSV+KNe9nYE60CgYBH
    v0pt2l+Rxp0gs7He1xMv/3J5PDOMChHdUpzzhMX+8I5vZXg6o0VbYYTTbuMTp1T4
    JiRwl0cdT8kl2plhJZP56naVH5cXWUnRygwZj2tPoZC3RxKOnrCdtSlgOBk2BmFM
    mbXRFzA8u/MOGUqCm7zPj0S5hbdM8ibSzfTki/mAAQKBgQDghA3DbU96L4neWcT0
    smth71RiuZzwb1wQsfN0Q1r8tQLz0qaRCefQiq0vlxaDmV7GDEZAz/KQFyC1+Egw
    hgyjzsaRaDN7er5XpWI2ug97k4xL5YVabtBTjW1Hc+DSWdLxioZk2PqU7zM4rAbJ
    Whxd/D5tfZD1aNGdLNlzkuBirg==
    -----END PRIVATE KEY-----
---
# Same as tls.crt to use in BackendTLSPolicy CA validation
apiVersion: v1
kind: ConfigMap
metadata:
  name: httpbin-tls-ca
data:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDDjCCAfagAwIBAgIUDcHvOZe5mvJVNTn3yB+m0K/EQkQwDQYJKoZIhvcNAQEL
    BQAwHjEcMBoGA1UEAwwTYmFja2VuZC5leGFtcGxlLmNvbTAeFw0yNTEyMDEyMjU0
    MzNaFw0zNTExMjkyMjU0MzNaMB4xHDAaBgNVBAMME2JhY2tlbmQuZXhhbXBsZS5j
    b20wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDBQ9MdW1HRb0kZZbsx
    DpO4xUmSPIs2M/8qovM+7/jnQndAs/j3hQGIUTGoBI4YsdmsAcHvxue7YnO4s0ZC
    xDRu1TuVAKWpgBGeBezIr0Qb4nszbPqj1+nvNhpWq7Jn8PZD06Kay0mlu82F3LDU
    VL3EePSqJmRvpYt6Y7bY4cZ1MR5Yzyb2pcRR5ueHEG38oVsOgxtJ6tKbS0byL9m/
    3AZVohrK2d9uY/Oij10GWjM7nuY4YKsMVxuZVJnx3oD8kfSFn0eBGs5WT4BZjDCM
    op3xxu7A1eFRrtv+4b06P9IMq9AOjaOYFMU1DZtUyrOToIS/o9ik4DiHR3zjcKGH
    k6UtAgMBAAGjRDBCMB4GA1UdEQQXMBWCE2JhY2tlbmQuZXhhbXBsZS5jb20wCwYD
    VR0PBAQDAgeAMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA0GCSqGSIb3DQEBCwUAA4IB
    AQARwvHK0fYaVeNK7T4h+V+dvi1ZDkCR70WEp8gRB94XiNu7InKnrfzFNDL5nz9Q
    NMu34XNGb6KIwTJTKC1zK+e+oYT9iVgQFvweYtnXmmGPDsJTufWmU90h4/dS2wnC
    tVNm1anyUCy7zw8ofWLKoVmMTZ5cLA0n+3mzIa1xiHv/aybZsGKVh+xFrlVg3xNu
    85jdn3CYv47i0+F2wbMoJbw4/G2pBgq89XWKUI01TZtzaJY2Nwrk3ZS61JY0YGO4
    YD5O7RK1O9jBdp83IwEZs8nohH4yUGfR06yiSFoIoxUP+NfcCdwI6OqvBJYWscZ6
    3hcwvzgG/lOYfUuzkVYOThwr
    -----END CERTIFICATE-----
---
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: httpbin-tls
spec:
  targetRefs:
  - group: ""
    kind: Service
    name: httpbin
  validation:
    hostname: backend.example.com
    caCertificateRefs:
    - group: ""
      kind: ConfigMap
      name: httpbin-tls-ca
EOF
```

## 4. Configure the OIDC Provider in kgateway

Next, configure kgateway with your OIDC provider's details. This involves creating a `GatewayExtension` to hold the OIDC configuration, a `Backend` and `BackendTLSPolicy` to allow kgateway to communicate with Google's OIDC endpoints, and a `Secret` for your client secret.

Apply the following manifest, making sure to replace the placeholder values.

```bash
kubectl apply -f- <<EOF
apiVersion: gateway.kgateway.dev/v1alpha1
kind: GatewayExtension
metadata:
  name: google-oauth
spec:
  oauth2:
    backendRef:
      kind: Backend
      group: gateway.kgateway.dev
      name: google-oauth
    issuerURI: https://accounts.google.com
    # tokenEndpoint and authorizationEndpoint can be omitted to use OpenID provider config discovery using the issuerURI
    #tokenEndpoint: https://oauth2.googleapis.com/token
    #authorizationEndpoint: https://accounts.google.com/o/oauth2/v2/auth
    credentials:
      # FIXME: replace with your OAuth2 client ID
      clientID: your-client-id
      clientSecretRef:
        name: google-oauth-client-secret
    redirectURI: https://example.com:8443/oauth2/redirect
    logoutPath: /logout
    forwardAccessToken: true
    scopes: ["openid", "email"]
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: google-oauth
spec:
  type: Static
  static:
    hosts:
    - host: oauth2.googleapis.com
      port: 443
---
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: google-oauth-tls
spec:
  targetRefs:
  - group: gateway.kgateway.dev
    kind: Backend
    name: google-oauth
  validation:
    hostname: oauth2.googleapis.com
    wellKnownCACertificates: System
---
apiVersion: v1
kind: Secret
metadata:
  name: google-oauth-client-secret
data:
  # FIXME: replace with your base64 encoded OAuth2 client secret
  client-secret: Y2xpZW50LXNlY3JldA==
EOF
```

**Note:** Make sure to replace `your-client-id` with your actual Client ID. For the `client-secret`, you must base64 encode your Client Secret and replace the value in the `google-oauth-client-secret` Secret. You can base64 encode your secret with: `echo -n 'your-client-secret' | base64`.

## 5. Expose and Protect the Application

Now, create a `Gateway` to expose a port to the internet and an `HTTPRoute` to route traffic for `example.com` to your `httpbin` backend. Then, use a `TrafficPolicy` to protect the route with the OIDC configuration you created in the previous step.

```bash
kubectl apply -f- <<EOF
# A self-signed certificate for example.com
apiVersion: v1
kind: Secret
metadata:
  name: example-tls
type: kubernetes.io/tls
data:
  tls.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM5akNDQWQ2Z0F3SUJBZ0lVUEhzK0hPVk9yaW9Ld2RYSnpKNG4rY3doa0c0d0RRWUpLb1pJaHZjTkFRRUwKQlFBd0ZqRVVNQklHQTFVRUF3d0xaWGhoYlhCc1pTNWpiMjB3SGhjTk1qVXhNakF4TVRZMU1qUXlXaGNOTXpVeApNVEk1TVRZMU1qUXlXakFXTVJRd0VnWURWUVFEREF0bGVHRnRjR3hsTG1OdmJUQ0NBU0l3RFFZSktvWklodmNOCkFRRUJCUUFEZ2dFUEFEQ0NBUW9DZ2dFQkFNQUNHZ1AyMEFDcFVWcnU5K3VrM1M5dkdWR1Q5bktDd3VPVG5JODgKdkpwOWNlaUdRSjdUZDRSb1VvNFM5UkhjT0svcTg1VWh4VHo1RzlacWtHNElqL3UrdURCM001ODdGR09aWUtTZQpJU0RZbVlhU0psM3FQWGtBcm8rNzVHU1pKMFhENUV2enZHd1FnZE5tTGp5RVNwbkhGL2dONitwdUhraDZhaFRSCmhRUDhHWkpKTm9NTStEUWNkYU5LSGxKY0lYdS9xTTdielpMeUtoank3dmhBd0tIa0FlZEJsTXVoUU5tdDFpdncKdUVUajZ3ZUhpazZDczAyU0Q3SFVHVjlRcHRTelpraTgvMEl1d1ZaWFVNcFNlMUZIOHZyS0FSaHZFT05YelN4aAo3Tms1aWIzd1RDT0tQMUFDTU9Oc0RSN042TnE2TU9nV055ZlpxSkwzUUtCbGhiVUNBd0VBQWFNOE1Eb3dGZ1lEClZSMFJCQTh3RFlJTFpYaGhiWEJzWlM1amIyMHdDd1lEVlIwUEJBUURBZ2VBTUJNR0ExVWRKUVFNTUFvR0NDc0cKQVFVRkJ3TUJNQTBHQ1NxR1NJYjNEUUVCQ3dVQUE0SUJBUUFuNkZmRFZ1U3VwSUREQzB2amtUWEU2TFdqM0JPUgpaMTlQQzVoZktkK0U2RnpPQ3U4TzRBdTVud0ZyS3Z0NWJZckpGcTRzdmRHWlpHNTlTL0ZhcDRvQ2JvYlV2a2krCnhsd1lXTktNOU1lUXJXVTgyTmFvOGxQK2NPUEh6azlOVENmNi9PbGNlZnFKRnI1TEFNWG5OZVc1cit0MlNkL2UKRkg3QlgyZnFtM0U0S1VWRGEzWlZQMUF1UStvaDB1WFlCTmlpaEMxWEZBT2NBdE9HK0VBR0xSeVVxek4xbjRQMQo5Z25HN1FaaTRXQ1V2MTdiT3J5akRJOVlEU09CbndRdU5MUTRyOWNTM2JQZmNUKzVqS2VZREJqeFlPWHVFYnBiCjFnbjB2ZEo0dFA4Z3ZiZ29KMEtWRlZ5S3JPM3MycWptQUVEbjNkbEFLc2pCSkNmS1Y2VUF4K1J6Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
  tls.key: LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUV2Z0lCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktnd2dnU2tBZ0VBQW9JQkFRREFBaG9EOXRBQXFWRmEKN3ZmcnBOMHZieGxSay9aeWdzTGprNXlQUEx5YWZYSG9oa0NlMDNlRWFGS09FdlVSM0RpdjZ2T1ZJY1U4K1J2VwphcEJ1Q0kvN3ZyZ3dkek9mT3hSam1XQ2tuaUVnMkptR2tpWmQ2ajE1QUs2UHUrUmttU2RGdytSTDg3eHNFSUhUClppNDhoRXFaeHhmNERldnFiaDVJZW1vVTBZVUQvQm1TU1RhRERQZzBISFdqU2g1U1hDRjd2NmpPMjgyUzhpb1kKOHU3NFFNQ2g1QUhuUVpUTG1VRFpyZFlyOExoRTQrc0hoNHBPZ3JOTmtnK3gxQmxmVUtiVXMyWkl2UDlDTHNGVwpWMURLVW50UlIvTDZ5Z0VZYnhEalY4MHNZZXpaT1ltOThFd2ppajlRQWpEamJBMGV6ZWphdWpEb0ZqY24yYWlTCjkwQ2daWVcxQWdNQkFBRUNnZ0VBYTBxQldRZTRzVFhyMVFGRm5mSncra2w5ZjExTDBDOExVZm13K1VVNktxWEEKV2V0eS9vMHg4dFlNazRFNldqR1JwNU9GYXljRXZRNkNKSzFGYVliMVZmbjdtSEZ6Y2gya1JnZDF2bWJ5SWhXRwpySERNYVp3em40TG5DRUE0M3BIS0pTelNUREsxYmpsSEltYXRuWGxhNmxVYktxdzAwTG1aeUd4SERMMExNKzdaCjlpRCsreUxJcDJiV3Q1dVROMGpTVEdIa1grb3NGdlhOSlg5aXZUQmhiMGdGcUJBdUN6VWE0TjRjTk56ZzRnV0sKa0MxMGsweUVOd2NhSTAxbzU2V1hZYUlFY2dwWVRORDZpQjA1bVBwZVJOKzJCZUVBdERYVUgxc0E4SGRCRHdRbApwVFltYmZLaHptVmlKeVMyc0hpK0VZbGN4eEZsYnUwcTNLTnh0aE5sWVFLQmdRRHE2b0lOY0dWWW9vamdWY1ptCm11SDc3cDYwUWdNTXBCd1JXb3lNUWFLbjFHd3ZEZGhHWWNDbHZJZ2pwVUlDbUFWckZtZksrSlc4WkhWc3BmOUIKSjBaVVJmaWQwUnBIaXk2ckhzNVduUlVIdmN0c0grRkkxZ2pMb2RzR1dHRm00SmQwOTYybUZOb2MyQ1laNmw2dgpVMDV5KzZtKzVUL3FOTXptT0g0bnlMRVVyUUtCZ1FEUlBidGxCYzdPZ2p4Z0pwVU9MUzd1ZW45bUlqSUhsQW1DCjN5NkpZWU9hL1ZDa1pZbWRvbTMrNDhwQ3d3UmZrdHlHUEQ1NnZ3Ly9WbDFUZUdJcDRiN3Q4c1owNm9wVFIrWGkKUlFyZXhiN2RoZGRRUGJMd0JYUFBFTk5UQlJMOUZuN0dQWXFGVUNXWklBc2VQdzVtODRJMS96bUJhSC9FczZ0bgpWTVlMMmI3T0tRS0JnUURqck9iZzJZY1AwVzh4WlZDRmp5VG9nOHRDenh1ZmU4cE1NMk0yYUVLWndESWRwS0J4CkRqcWxKc1VYTHdwNzh4U0ZSbERRRWY4bGVJT3FDblFLbEdNQU9GU050K1J0WklLVmpLVFVveWVIdWpYV2xFdEcKeVZINjhlS1NFc1JMN2U0OGlmTzluRVlNWUowRXp2WjNuQmpUTGYvRktQQzZML1JLU0lSVVVKajNmUUtCZ0FUdApKaWRYdnFuQUNUbmVUcTRaeER3YktEcTRYV011U2hjSnVDZkY0dnBZTW5qY1p5UU4rZmNCVi9iQWJxN3RYMEhOCjAwN0NodGJsS3FkWGMwQTNMMjZjdzYxbkJFQzN0YUxoSzBOWmRvZnlxY0lhNGNhaTZqb2ExRTdsRkxCZXdqZGEKcFpORDhzNnJJWGZoMWkzNFY3MTd0OWZqSlBiMW4vaDcxM25aODVNWkFvR0JBTUp6UitDUk0zMUU0VGErZlBLcQpJMk1vYmJEK2MvY2x1VTNLSWVPaFMreGx6blVuNGU5RDcreWdCWitqWTFMN0pheU9Xb3VLOEZoZWplOXZtZC82ClZlbzJ0REUwUzBZa1h5a0NnQ3FTamFYVHNZV0NNR0ZMUFhPd0lmaGhNczlZRnRUbWxJV0RZeXJobnBRQUVYUG8KS29vOUZDdzljNVlKOEdhUG4vQ0tudDN6Ci0tLS0tRU5EIFBSSVZBVEUgS0VZLS0tLS0K
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: oauth-gateway
spec:
  gatewayClassName: kgateway
  listeners:
  - name: https
    protocol: HTTPS
    port: 8443
    tls:
      mode: Terminate
      certificateRefs:
      - name: example-tls
        kind: Secret
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: oauth
spec:
  parentRefs:
  - name: oauth-gateway
  hostnames:
  - example.com
  rules:
  - name: rule0
    matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: httpbin
      port: 8443
---
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: oauth-route
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: oauth
  oauth2:
    extensionRef:
      name: google-oauth
EOF
```

> Note: the OAuth2 policy does not protect against Cross-Site-Request-Forgery attacks on domains with cached authentication (in the form of cookies). It is recommended to pair this with the CSRF policy to prevent malicious social engineering.

## 6. Configure DNS

Your browser needs to be able to resolve `example.com` to the `oauth-gateway`'s external IP address. For this demo, you can add an entry to your local `/etc/hosts` file.

1.  Get the external IP of the `oauth-gateway`:

    ```bash
    export GATEWAY_IP=$(kubectl get gateway oauth-gateway -o jsonpath='{.status.addresses[0].value}')
    ```

2.  Add an entry to `/etc/hosts`:

    ```bash
    echo "$GATEWAY_IP example.com" | sudo tee -a /etc/hosts
    ```

## 7. Access the Application

Now you can test the OIDC authentication flow.

1.  Open your browser and navigate to `https://example.com:8443`.
2.  You should be redirected to Google's login page.
3.  After successful authentication, you will be redirected back to the `httpbin` application.
