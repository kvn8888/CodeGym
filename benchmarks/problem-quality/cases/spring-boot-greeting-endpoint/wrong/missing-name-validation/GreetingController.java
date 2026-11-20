package com.codegym.exercise;

import java.util.Map;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class GreetingController {

    @GetMapping("/greeting")
    public ResponseEntity<?> greeting(@RequestParam(required = false) String name) {
        return ResponseEntity.ok(Map.of("message", "Hello, " + (name == null ? "" : name) + "!"));
    }
}
