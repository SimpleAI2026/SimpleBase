import java.util.*;

public class OrderManager {
    public List<String> outbox = new ArrayList<>();
    private Map<String, Integer> stock = new HashMap<>();
    private List<String> audit = new ArrayList<>();

    public OrderManager() { stock.put("apple", 5); stock.put("pear", 2); }

    // places an order, returns receipt text or error string
    public String placeOrder(String customer, String email, String item, int qty, String coupon) {
        if (customer == null || customer.trim().equals("")) { audit.add("REJECT no customer"); return "ERROR: customer"; }
        if (email == null || email.indexOf("@") < 0) { audit.add("REJECT bad email"); return "ERROR: email"; }
        if (!stock.containsKey(item)) { audit.add("REJECT unknown " + item); return "ERROR: item"; }
        if (qty <= 0 || stock.get(item) < qty) { audit.add("REJECT qty " + qty); return "ERROR: qty"; }
        int unit = 0;
        if (item.equals("apple")) unit = 120; else if (item.equals("pear")) unit = 250;
        int total = unit * qty;
        if (coupon != null && coupon.equals("TEN")) total = total - total / 10;
        if (coupon != null && coupon.equals("FLAT5")) total = Math.max(0, total - 500);
        stock.put(item, stock.get(item) - qty);
        String receipt = "Receipt for " + customer + ": " + qty + " x " + item + " = " + (total / 100) + "." + String.format("%02d", total % 100);
        outbox.add("TO:" + email + " BODY:" + receipt);
        audit.add("OK " + customer + " " + item + " " + qty);
        return receipt;
    }
    public List<String> getAudit() { return audit; }
    public int stockOf(String item) { return stock.getOrDefault(item, 0); }
}
