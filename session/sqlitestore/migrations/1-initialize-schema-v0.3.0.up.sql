-- Session

create table sessions_session (
  id                text    primary key,
  data              text    not null,
  creation_time     integer not null,
  modification_time integer not null,
  expiration_time   integer not null
) strict;

create index sessions_session_idx_id
on sessions_session (id);
