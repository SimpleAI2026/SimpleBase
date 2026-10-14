mod imp { include!(concat!(env!("IMPL"))); }
use imp::*;
fn main() {
    let m = parse_scores("a=1, b=5,a=7,,  ");
    assert_eq!(m["a"], 8); assert_eq!(m["b"], 5); assert_eq!(m.len(), 2);
    assert_eq!(best(&m), Some("a".to_string()));
    assert_eq!(best(&parse_scores("x=2,y=2")), Some("x".to_string()));
    assert_eq!(best(&parse_scores("z=0")), Some("z".to_string()));
    assert_eq!(best(&std::collections::HashMap::new()), None);
    for bad in ["a", "a=x", "a=-1", "a=1=2x"] {
        let r = std::panic::catch_unwind(|| parse_scores(bad));
        if bad == "a=1=2x" { assert!(r.is_ok()); } else { assert!(r.is_err(), "must panic on {bad}"); }
    }
    println!("PASS");
}
