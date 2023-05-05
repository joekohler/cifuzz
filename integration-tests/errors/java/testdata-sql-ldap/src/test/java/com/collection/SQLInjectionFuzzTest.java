package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;

import java.sql.Connection;
import java.sql.SQLException;

import com.code_intelligence.jazzer.junit.FuzzTest;
import org.h2.jdbcx.JdbcDataSource;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;

public class SQLInjectionFuzzTest {
    static Connection conn = null;

    void insecureInsertUser(String userName) throws SQLException {
        conn.createStatement().execute(String.format("INSERT INTO pet (name) VALUES ('%s')", userName));
    }

    @BeforeAll
    static void setupDB() throws Exception {
        JdbcDataSource ds = new JdbcDataSource();
        ds.setURL("jdbc:h2:./test.db");
        conn = ds.getConnection();
        conn.createStatement().execute(
                "CREATE TABLE IF NOT EXISTS pet (id IDENTITY PRIMARY KEY, name VARCHAR(50))");
    }

    @AfterAll
    static void removeDBFiles() {
        //TODO
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) throws Exception {
        insecureInsertUser(data.consumeRemainingAsString());
    }
}
