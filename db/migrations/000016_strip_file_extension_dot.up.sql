-- A new version of a file stored its extension with the dot (".txt") until
-- #358, while an upload stored it without ("txt"). Clients join name + "." +
-- ext, so those rows read "notes..txt". Strip the leading dots; a row that was
-- written correctly does not match the WHERE and is left alone.
UPDATE files SET ext = ltrim(ext, '.') WHERE ext LIKE '.%';
