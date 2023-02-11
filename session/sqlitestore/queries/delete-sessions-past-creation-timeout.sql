delete from sessions_session
where sessions_session.creation_time + $expiration_timeout < $expiration_time_threshold
