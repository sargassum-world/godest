delete from sessions_session
where sessions_session.expiration_time < $expiration_time_threshold
