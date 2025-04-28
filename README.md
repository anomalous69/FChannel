## Migrating from FChannel0/FChannel-Server, or a commit older than 24th April 2025
Please read [doc/migration.md](doc/migration.md) to see any warnings and manual intervention required  
**This fork is currently unstable, more manual intervention may be required in the near future.**  
**Please use tag [0.2.0](/../../commit/0a4928a30b1294bd5320160a8cdb51104cfdeb31) or check [doc/migration.md](doc/migration.md) if running from master**


# About

FChannel is a
[libre](https://en.wikipedia.org/wiki/Free_and_open-source_software),
[self-hostable](https://en.wikipedia.org/wiki/Self-hosting_(web_services)),
[federated](https://en.wikipedia.org/wiki/Federation_(information_technology)),
[imageboard](https://en.wikipedia.org/wiki/Imageboard) platform that utilizes
[ActivityPub](https://activitypub.rocks/) to federate between other instances.

The primary instance used by this fork is: [https://usagi.reisen](https://usagi.reisen)

Any contributions or suggestions are appreciated.  
Best way to give immediate feedback is the Matrix: `#fchan:matrix.org`

## Development
To get started on hacking the code of FChannel, it is recommended you setup your
Git hooks by simply running `git config core.hooksPath .githooks`.

Before you make a pull request, ensure everything you changed works as expected,
and to fix errors reported by `go vet` and make your code better with
`staticcheck`.

### Nix
`shell.nix` is available for those who use direnv and Lorri.

## Server Installation and Configuration

### Minimum Server Requirements

- Go v1.23+
- PostgreSQL (pgcrypto extension required for user post deletion)
- ImageMagick
- exiv2

### Server Installation Instructions

- Ensure you have Golang installed and set a correct `GOPATH`
- `git clone` the software
- Copy `config-init` to `config/config-init` and change the values appropriately to reflect the instance.
- Create the database, username, and password for psql that is used in the `config` file.
- Build the server with `make`
- Start the server with `./fchan`.

### Customization
Extra links to external boards, websites, etc... can be appended to the board navigation header by modifying [views/partials/extboards.html](views/partials/extboards.html).  
[Example with two external boards](views/partials/extboards.html.example) 

### Local testing

When testing on a local env when setting the `instance` value in the config file you have to append the port number to the local address eg. `instance:localhost:3000` with `instanceport` also being set to the same port.

If you want to test federation between servers locally you have to use your local ip as the `instance` eg. `instance:192.168.0.2:3000` and `instance:192.168.0.2:4000` adding the port to localhost will not route correctly.

### Managing the server

To access the managment page to create new boards or subscribe to other boards, when you start the server the console will output the `Mod key` and `Admin Login`
Use the `Mod key` by appending it to your server's url, `https://fchan.xyz/[Mod key]` once there you will be prompted for the `Admin Login` credentials.
You can manage each board by appending the `Mod key` to the desired board url: `https://fchan.xyz/[Mod Key]/g`
The `Mod key` is not static and is reset on server restart.

## Server Update

Check the git repo for the latest commits. If there are commits you want to update to, git pull and restart the instance.

## Networking

### NGINX Template

Use [Certbot](https://github.com/certbot/certbot), (or your tool of choice) to setup SSL.

```
server {
        listen 80;
        listen [::]:80;

        root /var/www/html;

        server_name DOMAIN_NAME;

        client_max_body_size 100M;

        location / {
                # First attempt to serve request as file, then
                # as directory, then fall back to displaying a 404.
                #try_files $uri $uri/ =404;
                proxy_pass http://localhost:3000;
                proxy_http_version 1.1;
                proxy_set_header Upgrade $http_upgrade;
                proxy_set_header Connection 'upgrade';
                proxy_set_header Host $host;
                proxy_set_header X-Real-IP $remote_addr;
                proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
                proxy_set_header X-Forwarded-Proto $scheme;
                proxy_cache_bypass $http_upgrade;
        }
}
```

#### Using Certbot With NGINX

- After installing Certbot and the Nginx plugin, generate the certificate: `sudo certbot --nginx --agree-tos --redirect --rsa-key-size 4096 --hsts --staple-ocsp --email YOUR_EMAIL -d DOMAIN_NAME`
- Add a job to cron so the certificate will be renewed automatically: `echo "0 0 * * *  root  certbot renew --quiet --no-self-upgrade --post-hook 'systemctl reload nginx'" | sudo tee -a /etc/cron.d/renew_certbot`

### Apache

`Please consider submitting a pull request if you set up a FChannel instance with Apache with instructions on how to do so`

### Caddy

`Please consider submitting a pull request if you set up a FChannel instance with Caddy with instructions on how to do so`

Remember, you may need to look at [the section on local testing](#local-testing)
to use 100% of FChannel's features.
