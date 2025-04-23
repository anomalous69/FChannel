CREATE TABLE IF NOT EXISTS actor(
type varchar(50) default '',
id varchar(100) UNIQUE PRIMARY KEY,
name varchar(50) default '',
preferredusername varchar(100) default '',
summary varchar(200) default '',
inbox varchar(100) default '',
outbox varchar(100) default '',
following varchar(100) default '',
followers varchar(100) default '',
restricted boolean default false
);

CREATE TABLE IF NOT EXISTS replies(
id varchar(100),
inreplyto varchar(100)
);

CREATE TABLE IF NOT EXISTS following(
id varchar(100),
following varchar(100)
);

CREATE TABLE IF NOT EXISTS follower(
id varchar(100),
follower varchar(100)
);

CREATE TABLE IF NOT EXISTS verification(
type varchar(50) default '',
identifier varchar(100),
code varchar(50),
created TIMESTAMP default NOW()
);

CREATE TABLE IF NOT EXISTS reported(
id varchar(100),
count int,
board varchar(100),
reason varchar(100)
);

CREATE TABLE IF NOT EXISTS verificationcooldown(
code varchar(50),
created TIMESTAMP default NOW()
);

CREATE TABLE IF NOT EXISTS boardaccess(
identifier varchar(100),
code varchar(50),
board varchar(100),
type varchar(50)
);

CREATE TABLE IF NOT EXISTS crossverification(
verificationcode varchar(50),
code varchar(50)
);

CREATE TABLE IF NOT EXISTS actorauth(
type varchar(50),
board varchar(100)
);

CREATE TABLE IF NOT EXISTS activitystream(
actor varchar(100) default '',
attachment varchar(100) default '',
attributedTo varchar(100) default '',
audience varchar(100) default '',
bcc varchar(100) default '',
bto varchar(100) default '',
cc varchar(100) default '',
context varchar(100) default '',
current varchar(100) default '',
first varchar(100) default '',
generator varchar(100) default '',
icon varchar(100) default '',
id varchar(100) UNIQUE PRIMARY KEY,
image varchar(100) default '',
instrument varchar(100) default '',
last varchar(100) default '',
location varchar(100) default '',
items varchar(100) default '',
oneOf varchar(100) default '',
anyOf varchar(100) default '',
closed varchar(100) default '',
origin varchar(100) default '',
next varchar(100) default '',
object varchar(100),
prev varchar(100) default '',
preview varchar(100) default '',
result varchar(100) default '',
tag varchar(100) default '',
target varchar(100) default '',
type varchar(100) default '',
to_ varchar(100) default '',
url varchar(100) default '',
accuracy varchar(100) default '',
altitude varchar(100) default '',
content varchar(2000) default '',
name varchar(256) default '',
alias varchar(100) default '',
duration varchar(100) default '',
height varchar(100) default '',
href varchar(100) default '',
hreflang varchar(100) default '',
partOf varchar(100) default '',
latitude varchar(100) default '',
longitude varchar(100) default '',
mediaType varchar(100) default '',
endTime varchar(100) default '',
published TIMESTAMP default NOW(),
startTime varchar(100) default '',
radius varchar(100) default '',
rel varchar(100) default '',
startIndex varchar(100) default '',
summary varchar(100) default '',
totalItems varchar(100) default '',
units varchar(100) default '',
updated TIMESTAMP default NOW(),
deleted TIMESTAMP default NULL,
width varchar(100) default '',
subject varchar(100) default '',
relationship varchar(100) default '',
describes varchar(100) default '',
formerType varchar(100) default '',
size int default NULL,
public boolean default false,
CONSTRAINT fk_object FOREIGN KEY (object) REFERENCES activitystream(id)
);

CREATE TABLE IF NOT EXISTS cacheactivitystream(
actor varchar(100) default '',
attachment varchar(100) default '',
attributedTo varchar(100) default '',
audience varchar(100) default '',
bcc varchar(100) default '',
bto varchar(100) default '',
cc varchar(100) default '',
context varchar(100) default '',
current varchar(100) default '',
first varchar(100) default '',
generator varchar(100) default '',
icon varchar(100) default '',
id varchar(100) UNIQUE PRIMARY KEY,
image varchar(100) default '',
instrument varchar(100) default '',
last varchar(100) default '',
location varchar(100) default '',
items varchar(100) default '',
oneOf varchar(100) default '',
anyOf varchar(100) default '',
closed varchar(100) default '',
origin varchar(100) default '',
next varchar(100) default '',
object varchar(100),
prev varchar(100) default '',
preview varchar(100) default '',
result varchar(100) default '',
tag varchar(100) default '',
target varchar(100) default '',
type varchar(100) default '',
to_ varchar(100) default '',
url varchar(100) default '',
accuracy varchar(100) default '',
altitude varchar(100) default '',
content varchar(2000) default '',
name varchar(256) default '',
alias varchar(100) default '',
duration varchar(100) default '',
height varchar(100) default '',
href varchar(100) default '',
hreflang varchar(100) default '',
partOf varchar(100) default '',
latitude varchar(100) default '',
longitude varchar(100) default '',
mediaType varchar(100) default '',
endTime varchar(100) default '',
published TIMESTAMP default NOW(),
startTime varchar(100) default '',
radius varchar(100) default '',
rel varchar(100) default '',
startIndex varchar(100) default '',
summary varchar(100) default '',
totalItems varchar(100) default '',
units varchar(100) default '',
updated TIMESTAMP default NOW(),
deleted TIMESTAMP default NULL,
width varchar(100) default '',
subject varchar(100) default '',
relationship varchar(100) default '',
describes varchar(100) default '',
formerType varchar(100) default '',
size int default NULL,
public boolean default false,
CONSTRAINT fk_object FOREIGN KEY (object) REFERENCES cacheactivitystream(id)
);

CREATE TABLE IF NOT EXISTS removed(
id varchar(100),
type varchar(25)
);

ALTER TABLE activitystream ADD COLUMN IF NOT EXISTS tripcode varchar(50) default '';
ALTER TABLE cacheactivitystream ADD COLUMN IF NOT EXISTS tripcode varchar(50) default '';

CREATE TABLE IF NOT EXISTS publicKeyPem(
id varchar(100) UNIQUE,
owner varchar(100),
file varchar(100)
);

CREATE TABLE IF NOT EXISTS newsItem(
title text,
content text,
time bigint
);

ALTER TABLE actor ADD COLUMN IF NOT EXISTS publicKeyPem varchar(100) default '';

ALTER TABLE activitystream ADD COLUMN IF NOT EXISTS sensitive boolean default false;
ALTER TABLE cacheactivitystream ADD COLUMN IF NOT EXISTS sensitive boolean default false;

CREATE TABLE IF NOT EXISTS postblacklist(
id serial primary key,
regex varchar(200)
);

ALTER TABLE actor ADD COLUMN IF NOT EXISTS autosubscribe boolean default false;


CREATE TABLE IF NOT EXISTS bannedimages(
phash numeric UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS bannedips (
ip inet NOT NULL,
reason varchar(512),
date timestamp DEFAULT timezone('utc', now()) NOT NULL,
expires timestamp DEFAULT '9999-12-31 00:00:00' NOT NULL
);

CREATE TABLE IF NOT EXISTS bannedmedia(
id serial primary key,
hash varchar(200)
);

CREATE TABLE IF NOT EXISTS inactive(
instance varchar(100) primary key,
timestamp TIMESTAMP default NOW()
);

ALTER TABLE boardaccess ADD COLUMN IF NOT EXISTS label varchar(50) default 'Anon';

CREATE TABLE IF NOT EXISTS sticky(
actor_id varchar(100),
activity_id varchar(100)
);

CREATE TABLE IF NOT EXISTS locked(
actor_id varchar(100),
activity_id varchar(100)
);

CREATE TABLE IF NOT EXISTS identify (
id varchar(256) NOT NULL,
ip inet,
password character varying,
posted timestamp DEFAULT timezone('utc', now())
);

--ALTER TABLE ONLY identify ADD CONSTRAINT IF NOT EXISTS identify_id_fkey FOREIGN KEY (id) REFERENCES activitystream(id) ON UPDATE CASCADE ON DELETE CASCADE NOT DEFERRABLE;
ALTER TABLE activitystream ALTER COLUMN content TYPE varchar(4500);
ALTER TABLE cacheactivitystream ALTER COLUMN content TYPE varchar(4500);

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE actor ADD COLUMN IF NOT EXISTS boardtype varchar default 'image';

DROP TABLE IF EXISTS wallet;