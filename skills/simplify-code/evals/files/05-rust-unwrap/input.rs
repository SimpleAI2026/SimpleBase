use std::collections::HashMap;

/// Public API used by other crates: parses "name=score,name=score".
/// Panics on malformed input (documented behavior callers rely on in tests).
pub fn parse_scores(s: &str) -> HashMap<String, u32> {
    let mut m = HashMap::new();
    for pair in s.split(',') {
        if pair.trim().len() == 0 { continue; }
        let mut it = pair.split('=');
        let name = it.next().unwrap().trim().to_string();
        let score = it.next().unwrap().trim().parse::<u32>().unwrap();
        if m.contains_key(&name) {
            let old = *m.get(&name).unwrap();
            m.insert(name.clone(), old + score);
        } else {
            m.insert(name.clone(), score);
        }
    }
    m
}

pub fn best(m: &HashMap<String, u32>) -> Option<String> {
    let mut best: Option<String> = None;
    let mut best_score = 0;
    let mut keys: Vec<&String> = m.keys().collect();
    keys.sort();
    for k in keys {
        let v = *m.get(k).unwrap();
        if best.is_none() || v > best_score { best = Some(k.clone()); best_score = v; }
    }
    best
}
