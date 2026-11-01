ALTER TABLE users ALTER COLUMN id DROP IDENTITY;
CREATE SEQUENCE users_id_seq as BIGINT OWNED BY users.id;

ALTER TABLE users ALTER COLUMN id SET DEFAULT nextval('users_id_seq');

SELECT setval(
   'users_id_seq', COALESCE(MAX(id), 0) + 1, false
) FROM users;
