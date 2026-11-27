package demo;

import java.util.List;

/** A service. */
public class Service {
    private static final String BRACE = "{";

    public Service(String name) {
        this.name = name;
    }

    @Override
    public String toString() {
        return "Service{" + name + "}";
    }

    public <T> List<T> find(Class<T> type, int limit) {
        if (limit > 0) {
            return null;
        }
        return List.of();
    }

    public void find(String q) {}

    abstract void run();

    static class Inner {
        void go() {
            helper();
        }
    }
}

interface Shape {
    double area();
}
