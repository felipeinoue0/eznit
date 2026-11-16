-- Nothing to reverse: the dotted extension was the bug (#358), and which rows
-- carried it is not recorded. A no-op keeps the pair symmetrical for the
-- migrate tool.
SELECT 1;
