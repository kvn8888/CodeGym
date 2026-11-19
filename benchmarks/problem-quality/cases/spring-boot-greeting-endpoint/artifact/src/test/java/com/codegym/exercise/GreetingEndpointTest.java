package com.codegym.exercise;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Iterator;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;
import org.junit.jupiter.api.Timeout;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.web.context.WebServerApplicationContext;
import org.springframework.context.ConfigurableApplicationContext;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
@Timeout(value = 90, unit = TimeUnit.SECONDS)
class GreetingEndpointTest {

    private static final String PROTOCOL = "CODEGYM_TEST";
    private static final int STARTUP_TIMEOUT_SECONDS = 30;
    private static final Duration CONNECT_TIMEOUT = Duration.ofSeconds(3);
    private static final Duration REQUEST_TIMEOUT = Duration.ofSeconds(3);
    private static final ObjectMapper JSON = new ObjectMapper();

    private ConfigurableApplicationContext context;
    private HttpClient client;
    private int port;
    private int failures;

    @BeforeAll
    void startServer() throws Exception {
        SpringApplication application = new SpringApplication(GreetingApplication.class);
        application.setDefaultProperties(Map.<String, Object>of(
                "server.address", "127.0.0.1",
                "server.port", "0",
                "spring.main.banner-mode", "off",
                "logging.level.root", "ERROR"));

        ExecutorService executor = Executors.newSingleThreadExecutor(runnable -> {
            Thread thread = new Thread(runnable, "greeting-test-startup");
            thread.setDaemon(true);
            return thread;
        });
        Future<ConfigurableApplicationContext> started = executor.submit(() -> application.run());
        try {
            context = started.get(STARTUP_TIMEOUT_SECONDS, TimeUnit.SECONDS);
        } catch (TimeoutException error) {
            started.cancel(true);
            throw new IllegalStateException(
                    "Spring Boot startup exceeded " + STARTUP_TIMEOUT_SECONDS + " seconds", error);
        } finally {
            executor.shutdownNow();
        }

        port = ((WebServerApplicationContext) context).getWebServer().getPort();
        client = HttpClient.newBuilder().connectTimeout(CONNECT_TIMEOUT).build();
    }

    @AfterAll
    void stopServer() {
        if (context != null) {
            context.close();
            context = null;
        }
    }

    @BeforeEach
    void resetFailures() {
        failures = 0;
    }

    @AfterEach
    void failWhenAssertionsFailed() {
        if (failures > 0) {
            Assertions.fail(failures + " greeting assertion(s) failed");
        }
    }

    @Test
    void hiddenSuccess() throws Exception {
        Response response = get("/greeting?name=Ada");
        if (response.status() != 200) {
            check("hidden-success", false, "HTTP_STATUS", 200, response.status());
            return;
        }
        check("hidden-success", isJson(response), "CONTENT_TYPE", "application/json", response.contentType());
    }

    @Test
    void hiddenResponseShape() throws Exception {
        Response response = get("/greeting?name=Ada");
        if (response.status() != 200) {
            check("hidden-response-shape", false, "HTTP_STATUS", 200, response.status());
            return;
        }
        JsonNode body = json(response);
        if (body == null || !body.has("message")) {
            check("hidden-response-shape", false, "JSON_FIELD", "message", firstField(body));
            return;
        }
        check("hidden-response-shape", "Hello, Ada!".equals(body.get("message").asText()),
                "JSON_VALUE", "Hello, Ada!", body.get("message").asText());
    }

    @Test
    void hiddenMissingName() throws Exception {
        Response response = get("/greeting");
        if (response.status() != 400) {
            check("hidden-missing-name", false, "HTTP_STATUS", 400, response.status());
            return;
        }
        if (!isJson(response)) {
            check("hidden-missing-name", false, "CONTENT_TYPE", "application/json", response.contentType());
            return;
        }
        JsonNode body = json(response);
        check("hidden-missing-name", body != null && "name is required".equals(body.path("error").asText()),
                "JSON_VALUE", "name is required", body == null ? null : body.path("error").asText(null));
    }

    @Test
    void hiddenBlankName() throws Exception {
        Response response = get("/greeting?name=%20%20%20");
        if (response.status() != 400) {
            check("hidden-blank-name", false, "HTTP_STATUS", 400, response.status());
            return;
        }
        JsonNode body = json(response);
        check("hidden-blank-name", body != null && "name is required".equals(body.path("error").asText()),
                "JSON_VALUE", "name is required", body == null ? null : body.path("error").asText(null));
    }

    @Test
    void hiddenUnknownQuery() throws Exception {
        Response response = get("/greeting?name=Ada&extra=ignored");
        if (response.status() != 200) {
            check("hidden-unknown-query", false, "HTTP_STATUS", 200, response.status());
            return;
        }
        JsonNode body = json(response);
        check("hidden-unknown-query", body != null && "Hello, Ada!".equals(body.path("message").asText()),
                "JSON_VALUE", "Hello, Ada!", body == null ? null : body.path("message").asText(null));
    }

    @Test
    void hiddenNoTrimming() throws Exception {
        Response response = get("/greeting?name=%20Ada%20");
        if (response.status() != 200) {
            check("hidden-no-trimming", false, "HTTP_STATUS", 200, response.status());
            return;
        }
        JsonNode body = json(response);
        check("hidden-no-trimming", body != null && "Hello,  Ada !".equals(body.path("message").asText()),
                "JSON_VALUE", "Hello,  Ada !", body == null ? null : body.path("message").asText(null));
    }

    private Response get(String path) throws Exception {
        HttpRequest request = HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path))
                .timeout(REQUEST_TIMEOUT)
                .header("Accept", "application/json")
                .GET()
                .build();
        HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());
        return new Response(
                response.statusCode(),
                response.headers().firstValue("content-type").orElse(""),
                response.body());
    }

    private static boolean isJson(Response response) {
        return response.contentType().toLowerCase().contains("application/json");
    }

    private static JsonNode json(Response response) {
        try {
            return JSON.readTree(response.body());
        } catch (Exception error) {
            return null;
        }
    }

    private static String firstField(JsonNode body) {
        if (body == null || !body.isObject()) {
            return null;
        }
        Iterator<String> names = body.fieldNames();
        return names.hasNext() ? names.next() : null;
    }

    private void check(String testId, boolean passed, String code, Object expected, Object actual) {
        if (passed) {
            System.out.println(PROTOCOL + " " + testId + " PASS");
            return;
        }
        failures++;
        System.out.println(PROTOCOL + " " + testId + " FAIL code=" + code
                + " expected=" + protocolValue(expected)
                + " actual=" + protocolValue(actual));
    }

    private static String protocolValue(Object value) {
        if (value == null) {
            return "(missing)";
        }
        String text = String.valueOf(value).trim().replaceAll("\\s+", "_");
        return text.isEmpty() ? "(empty)" : text;
    }

    private record Response(int status, String contentType, String body) {}
}
