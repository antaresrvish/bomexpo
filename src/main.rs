use reqwest;

#[tokio::main]
async fn main() {
    let url = String::from("https://lcsc.com/product-detail/C845035");
    let body = test_get(url).await;
    println!("body = {body:?}");
}

async fn test_get(x: String) -> Result<String, reqwest::Error> {
    reqwest::get(x).await?.text().await
}
