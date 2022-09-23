update sessions_session
set
  data = $data,
  modification_time = $modification_time
where sessions_session.id = $id
