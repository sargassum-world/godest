select
  s.id                as id,
  s.data              as data,
  s.creation_time     as creation_time,
  s.modification_time as modification_time,
  s.expiration_time   as expiration_time
from sessions_session as s
where
  s.id = $id
