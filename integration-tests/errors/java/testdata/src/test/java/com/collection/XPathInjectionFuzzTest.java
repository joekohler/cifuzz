package com.collection;

import java.io.*;
import javax.xml.parsers.*;
import javax.xml.xpath.*;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;
import org.junit.jupiter.api.BeforeAll;
import org.w3c.dom.Document;
import org.xml.sax.*;

public class XPathInjectionFuzzTest {
    static Document doc = null;
    static XPath xpath = null;
    
    @BeforeAll
    static void initialize() throws Exception {
        String xmlFile = "<user name=\"user\" pass=\"pass\"></user>";

        DocumentBuilderFactory domFactory = DocumentBuilderFactory.newInstance();
        domFactory.setNamespaceAware(true);
        DocumentBuilder builder = domFactory.newDocumentBuilder();
        doc = builder.parse(new InputSource(new StringReader(xmlFile)));

        XPathFactory xpathFactory = XPathFactory.newInstance();
        xpath = xpathFactory.newXPath();
    }
    
    void xPath(String user, String pass) {
                if (user != null && pass != null) {
            String expression = "/user[@name='" + user + "' and @pass='" + pass + "']";
            try {
                xpath.evaluate(expression, doc, XPathConstants.BOOLEAN);
            } catch (XPathExpressionException e) {
            }
        }
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        xPath(data.consumeString(20), data.consumeRemainingAsString());
    }
}
