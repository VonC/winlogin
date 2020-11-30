# winlogin: get Windows login from name or email

This Go program is for Windows only.  
It uses the environment variable `USERMAIL` to:

- determine the mail domain (`@company.com`)
- make [`DSQUERY` commands](https://serverfault.com/a/576634/783) to get the Windows login from a name or an email

## Why

I often need to add Windows login at the request of a user, for themselves or several other users.  
But I only get their name or emails.

Rather than writing them back, asking for their Windows login (which does not always follow a clear convention), I query the Active Directory, looking for an AD antry matching a user name or email

`DSQUERY * domainroot -filter "(&(objectCategory=Person)(objectClass=User)(mail=%1))" -attr sAMAccountName`

## From name

Launch `winlogin`, and start typing the first letter of the name.

The very first letter must be the first from either the first or last name searched.  
After that, any other letter can be non-sequential.  
You can add a space, to separate first/last names (or last/first names: the AD query will check both)

Once there is only one login matching the user firstname/lastname, `winlogin` copies the login in the clipboard and exits automatically.

Example:

```
# Users

Bob Martinhood
Mike Robertson
```

- Typing `mb` or `mk` would return `Mike Robertson`:  
`m` is the first letter of the first name, then '`b`' is any letter after `m` (in the first or lastname).
- Typing `m b` would return `Bob Martinhood`:  
'm' would be either the first letter of the first name or of the lastname, same for `b`.

So adding a space between `rb mk` forces `winlogin` to consider any entry with:

- a firstname starting with `m`, including `k`
- a lastname starting with `r`, and including `b`
- or the reverse (inverse firstname and lastname)

Typing `rbmk` (without space) would not select any entry (none start with `r`, and include in non-sequential order the letter `b`, `m`, `k`)

## From file

Drop a file with emails in it, and call `winlogin <myFile>`

All emails matching your email domain (from `%USERMAIL%`) will be extracted, and their login will be displayed.
