package com.appcontrol.transport;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;

/** API 28-compatible bounded read. Ownership and response-size validation stay with the caller. */
public final class BoundedInput {
    private BoundedInput() { }

    public static byte[] read(InputStream input, int maximum) throws IOException {
        if (maximum < 0) throw new IllegalArgumentException("Negative input bound");
        ByteArrayOutputStream output = new ByteArrayOutputStream(Math.min(maximum, 8192));
        byte[] buffer = new byte[Math.min(maximum, 8192)];
        while (output.size() < maximum) {
            int count = input.read(buffer, 0, Math.min(buffer.length, maximum - output.size()));
            if (count < 0) break;
            if (count == 0) {
                int value = input.read();
                if (value < 0) break;
                output.write(value);
            } else output.write(buffer, 0, count);
        }
        return output.toByteArray();
    }
}
