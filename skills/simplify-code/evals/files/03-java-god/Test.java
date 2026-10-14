public class Test {
    public static void main(String[] a) {
        OrderManager m = new OrderManager();
        String[] r = {
            m.placeOrder("Ann", "a@x", "apple", 3, null),
            m.placeOrder("Bob", "b@x", "pear", 2, "TEN"),
            m.placeOrder("Cy", "c@x", "pear", 1, null),
            m.placeOrder(" ", "c@x", "apple", 1, null),
            m.placeOrder("Di", "nope", "apple", 1, null),
            m.placeOrder("Ed", "e@x", "kiwi", 1, null),
            m.placeOrder("Fy", "f@x", "apple", 0, null),
            m.placeOrder("Gu", "g@x", "apple", 2, "FLAT5"),
            m.placeOrder(null, null, null, 1, null),
        };
        String got = String.join("|", r) + "#" + m.getAudit() + "#" + m.outbox + "#" + m.stockOf("apple") + m.stockOf("pear");
        String exp = "Receipt for Ann: 3 x apple = 3.60|Receipt for Bob: 2 x pear = 4.50|ERROR: qty|ERROR: customer|ERROR: email|ERROR: item|ERROR: qty|Receipt for Gu: 2 x apple = 0.00|ERROR: customer#[OK Ann apple 3, OK Bob pear 2, REJECT qty 1, REJECT no customer, REJECT bad email, REJECT unknown kiwi, REJECT qty 0, OK Gu apple 2, REJECT no customer]#[TO:a@x BODY:Receipt for Ann: 3 x apple = 3.60, TO:b@x BODY:Receipt for Bob: 2 x pear = 4.50, TO:g@x BODY:Receipt for Gu: 2 x apple = 0.00]#00";
        if (got.equals(exp)) System.out.println("PASS"); else { System.out.println("FAIL\n" + got); System.exit(1); }
    }
}
