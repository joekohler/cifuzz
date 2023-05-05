package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;
import mocks.MockLDAPContext;

import javax.naming.directory.DirContext;
import javax.naming.directory.SearchControls;

@SuppressWarnings("BanJNDI")
// TODO: how to run them separately on cmd?
public class LDAPInjectionFuzzTest {
    private static final DirContext ctx = new MockLDAPContext();
    
    void dn(String ou) throws Exception {
        String base = "ou=" + ou + ",dc=example,dc=com";
        ctx.search(base, "(&(uid=foo)(cn=bar))", new SearchControls());
    }
    
    void search(String uid) throws Exception {
        String filter = "(&(uid=" + uid + ")(ou=security))";
        ctx.search("dc=example,dc=com", filter, new SearchControls());
    }

    @FuzzTest
    void dnFuzzTest(FuzzedDataProvider fuzzedDataProvider) throws Exception {
        dn(fuzzedDataProvider.consumeRemainingAsString());
    }

    @FuzzTest
    void searchFuzzTest(FuzzedDataProvider fuzzedDataProvider) throws Exception {
        search(fuzzedDataProvider.consumeRemainingAsAsciiString());
    }
}
