import java.util.*;

public class Index {
    // email -> display name; called from the import job
    public static Map<String, String> byEmail(List<String[]> users) {
        Map<String, String> m = new HashMap<>();
        for (int i = 0; i < users.size(); i++) {
            String[] u = users.get(i);
            if (u[1] != null && u[1].length() > 0) {
                m.put(u[1].toLowerCase(), u[0]);
            }
        }
        return m;
    }
}
