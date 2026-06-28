-- Record which team won a finished match. NULL until a participant ends the
-- DRAWN round and picks the winner (see session.Manager.Finish).
ALTER TABLE sessions ADD COLUMN winner_team TEXT; -- 'A' | 'B' | ... | NULL
