# This fork is currently unstable, please check for changes is this file before pulling new commits

## Migrating from FChannel0/FChannel-Server, or a commit older than 24th April 2025

This list is short now but will likely grow as time goes on.  
Care will be taken to make this as automatic as possible.  

1. **Make a backup of your database** (e.g. sudo -u postgres pg_dump database_name > fchan_backup.psql). 
2. Move `config/config-init` to `fchan.cfg`.