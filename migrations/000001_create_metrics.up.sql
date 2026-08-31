CREATE TABLE metrics (
    id varchar(255) NOT NULL,
    mtype varchar(16) NOT NULL,
    delta bigint,
    value double precision,
    PRIMARY KEY (id, mtype)
);
