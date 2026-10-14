import java.util.*;
public class Test {
    static void check(boolean c, String m) { if (!c) { System.out.println("FAIL " + m); System.exit(1); } }
    public static void main(String[] a) {
        List<String[]> us = new ArrayList<>();
        us.add(new String[]{"Ann", "A@x.com"});
        us.add(new String[]{"Bob", ""});
        us.add(new String[]{"Cy", null});
        us.add(new String[]{"Ann2", "a@X.com"}); // duplicate after lowercasing: last wins
        us.add(new String[]{null, "n@x.com"});   // null name allowed
        Map<String, String> m;
        try { m = Index.byEmail(us); } catch (Exception e) { System.out.println("FAIL threw " + e); System.exit(1); return; }
        check(m.size() == 2, "size " + m);
        check("Ann2".equals(m.get("a@x.com")), "last wins " + m);
        check(m.containsKey("n@x.com") && m.get("n@x.com") == null, "null value kept " + m);
        m.put("z", "z"); // must stay mutable
        check(Index.byEmail(new ArrayList<>()).isEmpty(), "empty");
        System.out.println("PASS");
    }
}
