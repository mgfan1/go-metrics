CREATE TABLE metrics (
    pk bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id varchar(255) NOT NULL,
    mtype varchar(16) NOT NULL,
    delta bigint NULL,
    value double precision NULL,
    UNIQUE (id, mtype)
);
