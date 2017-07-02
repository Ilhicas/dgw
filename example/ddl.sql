DROP TABLE if EXISTS t1;
DROP TABLE if EXISTS t2;
DROP TABLE if EXISTS t3;
DROP TABLE if EXISTS user_account;
DROP TABLE if EXISTS user_account_composite_pk;
DROP TABLE if EXISTS user_account_uuid;

CREATE TABLE user_account (
  id bigserial primary key
  , email text not null unique
  , last_name text not null
  , first_name text not null
);

CREATE TABLE user_account_uuid (
  uuid uuid primary key
  , email text not null unique
  , last_name text not null
  , first_name text not null
);

CREATE TABLE user_account_composite_pk (
  id bigint not null
  , email text not null
  , last_name text not null
  , first_name text not null
  , PRIMARY KEY(id, email)
);

CREATE TABLE t1 (
  id bigserial primary key
  , i integer not null unique
  , str text not null
  , num_float numeric not null
  , nullable_str text
  , t_with_tz timestamp without time zone not null
  , t_without_tz timestamp with time zone not null
  , nullable_tz timestamp with time zone
  , json_data json not null
  , xml_data xml not null
);

CREATE TABLE t2 (
  id bigserial not null
  , i integer not null
  , str text not null
  , t_with_tz timestamp without time zone not null
  , t_without_tz timestamp with time zone not null
  , PRIMARY KEY(id, i)
);

CREATE TABLE t3 (
  id integer not null
  , i integer not null
  , PRIMARY KEY(id, i)
);
