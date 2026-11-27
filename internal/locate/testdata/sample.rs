use std::fmt;

/// A point.
#[derive(Debug)]
pub struct Point {
    x: i32,
}

pub struct Unit;

impl Point {
    pub fn new(x: i32) -> Self {
        Point { x }
    }

    fn describe(&self) -> &'static str {
        let s = r#"fn fake() { "# ;
        "point"
    }
}

impl fmt::Display for Point {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.x)
    }
}

pub trait Shape {
    fn area(&self) -> f64;
}

pub async fn run<T>(x: T) -> T
where
    T: Clone,
{
    x
}

mod util {
    pub fn helper() {}
}
