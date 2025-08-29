# Local Webhook Provider

A simple webhook provider that modifies `/etc/hosts` localhost, mostly used for testing purposes. Feel free to use it for your own testing or at your own risk, but it is not intended nor tested for production use.

## Idea

The provider correctly handls A, AAAA and CNAME records. Whenever a call is made to the webhook, it will update the `/etc/hosts` file on the localhost with the new DNS records.
