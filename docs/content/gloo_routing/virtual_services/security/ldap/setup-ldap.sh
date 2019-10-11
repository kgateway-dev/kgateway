#!/usr/bin/env bash

####################################################################################################
# This script is used to deploy an LDAP server with sample user/group configuration to Kubernetes #
####################################################################################################
set -e

if [ -z "$1" ]; then
  echo "No namespace provided, using default namespace"
  NAMESPACE='default'
else
  NAMESPACE=$1
fi

echo "Creating configmap with LDAP server bootstrap config..."
kubectl apply -n "${NAMESPACE}" -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ldap
data:
  01_overlay.ldif: |-
    ######################################################################
    # Create a 'memberof' overlay for 'groupOfNames' entries.
    #
    # This will cause the 'memberOf' attribute to be automatically added
    # to user entries when they are referenced in a group entry.
    ######################################################################
    dn: olcOverlay={1}memberof,olcDatabase={1}mdb,cn=config
    objectClass: olcOverlayConfig
    objectClass: olcMemberOf
    olcOverlay: {1}memberof
    olcMemberOfDangling: ignore
    olcMemberOfRefInt: TRUE
    olcMemberOfGroupOC: groupOfNames
    olcMemberOfMemberAD: member
    olcMemberOfMemberOfAD: memberOf
  02_acl.ldif: |-
    dn: olcDatabase={1}mdb,cn=config
    changeType: modify
    ######################################################################
    # Delete default ACLs that come with Docker image
    ######################################################################
    delete: olcAccess
    olcAccess: to attrs=userPassword,shadowLastChange by self write by dn="cn=admin,dc=solo,dc=io" write by anonymous auth by * none
    -
    delete: olcAccess
    olcAccess: to * by self read by dn="cn=admin,dc=solo,dc=io" write by * none
    -
    ######################################################################
    # Control access to People
    ######################################################################
    add: olcAccess
    olcAccess: to dn.subtree="ou=people,dc=solo,dc=io"
        by dn="cn=admin,dc=solo,dc=io" write
        by group.exact="cn=managers,ou=groups,dc=solo,dc=io" write
        by group.exact="cn=developers,ou=groups,dc=solo,dc=io" read
        by group.exact="cn=sales,ou=groups,dc=solo,dc=io" read
        by anonymous auth
    -
    ######################################################################
    # Control access to Groups
    ######################################################################
    add: olcAccess
    olcAccess: to dn.subtree="ou=groups,dc=solo,dc=io"
        by dn="cn=admin,dc=solo,dc=io" write
        by group.exact="cn=managers,ou=groups,dc=solo,dc=io" write
        by group.exact="cn=developers,ou=groups,dc=solo,dc=io" write
    -
    ######################################################################
    # This policy applies to the 'userPassword' attribute only
    # 'self write' grants only the owner of the entry write permission to this attribute
    # 'anonymous auth' grants an anonymous user access to this attribute only for authentication purposes (required for BIND)
    # 'developers' group members can update any user's password
    ######################################################################
    add: olcAccess
    olcAccess: to attrs=userPassword
      by self write
      by anonymous auth
      by dn="cn=admin,dc=solo,dc=io"
      by group.exact="cn=developers,ou=groups,dc=solo,dc=io" write
      by * none
    -
    ######################################################################
    # This policy applies to all entries under the "dc=solo,dc=io" subtree
    # 'managers' have read access at all the organization's information
    ######################################################################
    add: olcAccess
    olcAccess: to dn.subtree="dc=solo,dc=io"
      by self write
      by dn="cn=admin,dc=solo,dc=io" write
      by group.exact="cn=managers,ou=groups,dc=solo,dc=io" read
      by * none
  03_people.ldif: |
    # Create a parent 'people' entry
    dn: ou=people,dc=solo,dc=io
    objectClass: organizationalUnit
    ou: people
    description: All solo.io people

    # Add 'marco'
    dn: uid=marco,ou=people,dc=solo,dc=io
    objectClass: inetOrgPerson
    cn: Marco Schmidt
    sn: Schmidt
    uid: marco
    userPassword: marcopwd
    mail: marco.schmidt@solo.io

    # Add 'rick'
    dn: uid=rick,ou=people,dc=solo,dc=io
    objectClass: inetOrgPerson
    cn: Rick Ducott
    sn: Ducott
    uid: rick
    userPassword: rickpwd
    mail: rick.ducott@solo.io

    # Add 'scottc'
    dn: uid=scottc,ou=people,dc=solo,dc=io
    objectClass: inetOrgPerson
    cn: Scott Cranton
    sn: Cranton
    uid: scottc
    userPassword: scottcpwd
    mail: scott.cranton@solo.io
  04_groups.ldif: |+
    # Create top level 'group' entry
    dn: ou=groups,dc=solo,dc=io
    objectClass: organizationalUnit
    ou: groups
    description: Generic parent entry for groups

    # Create the 'developers' entry under 'groups'
    dn: cn=developers,ou=groups,dc=solo,dc=io
    objectClass: groupOfNames
    cn: developers
    description: Developers group
    member: uid=marco,ou=people,dc=solo,dc=io
    member: uid=rick,ou=people,dc=solo,dc=io
    member: uid=scottc,ou=people,dc=solo,dc=io

    # Create the 'sales' entry under 'groups'
    dn: cn=sales,ou=groups,dc=solo,dc=io
    objectClass: groupOfNames
    cn: sales
    description: Sales group
    member: uid=scottc,ou=people,dc=solo,dc=io

    # Create the 'managers' entry under 'groups'
    dn: cn=managers,ou=groups,dc=solo,dc=io
    objectClass: groupOfNames
    cn: managers
    description: Managers group
    member: uid=rick,ou=people,dc=solo,dc=io
EOF

echo "Creating LDAP service and deployment..."
kubectl apply -n "${NAMESPACE}" -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: ldap
  name: ldap
spec:
  selector:
    matchLabels:
      app: ldap
  replicas: 1
  template:
    metadata:
      labels:
        app: ldap
    spec:
      volumes:
      - name: config
        emptyDir: {}
      - name: configmap
        configMap:
          name: ldap
      # We need this intermediary step because when Kubernetes mounts a configMap to a directory,
      # it generates additional files that the LDAP server tries to load, causing it to fail.
      initContainers:
      - name: copy-config
        image: busybox
        command: ['sh', '-c', 'cp /configmap/*.ldif /config']
        volumeMounts:
        - name: configmap
          mountPath: /configmap
          # This is the volume that will be mounted to the LDAP container
        - name: config
          mountPath: /config
      containers:
      - image: osixia/openldap:1.2.5
        name: openldap
        args: ["--copy-service", "--loglevel", "debug"]
        env:
        - name: LDAP_ORGANISATION
          value: "Solo.io"
        - name: LDAP_DOMAIN
          value: "solo.io"
        - name: LDAP_ADMIN_PASSWORD
          value: "solopwd"
        ports:
        - containerPort: 389
          name: ldap
        - containerPort: 636
          name: ldaps
        volumeMounts:
        - mountPath: /container/service/slapd/assets/config/bootstrap/ldif/custom
          name: config
---
apiVersion: v1
kind: Service
metadata:
  name: ldap
  labels:
    app: ldap
spec:
  ports:
  - port: 389
    protocol: TCP
  selector:
    app: ldap
EOF
