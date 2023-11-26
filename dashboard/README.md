# UI for [Fn](https://github.com/fnproject/fn) [![CircleCI](https://circleci.com/gh/fnproject/ui.svg?style=svg)](https://circleci.com/gh/fnproject/ui)

## Usage

Start an fn server

```sh
fn start
```

Start the UI:

```sh
docker run --rm -it --link fnserver:api -p 4000:4000 -e "FN_API_URL=http://api:8080" fnproject/ui
```

## Screenshots

All apps view:

<img src="https://raw.githubusercontent.com/fnproject/ui/master/docs/screenshots/apps.png" width="800">

All functions in an app:

<img src="https://raw.githubusercontent.com/fnproject/ui/master/docs/screenshots/routes.png" width="800">

## Development

### Setup

#### 1) Install dependencies

```sh
npm install

# if you want webpack globally
sudo npm install -g webpack@^1.14.0
```

#### 2) Start Functions API (see [Fn on GitHub](http://github.com/fnproject/fn))

```sh
fn start
```

#### 3) Compile assets

```sh
# option 1: if global webpack
webpack

# option 2: if local webpack
npx webpack

webpack --config webpack.config.cjs
```

#### 4) Start web server

```sh
PORT=4000 FN_API_URL=http://localhost:8080 npm start
```

* `PORT` - port to run UI on. Optional, 4000 by default
* `FN_API_URL` - Functions API URL. Required

#### 5) View in browser

[http://localhost:4000/](http://localhost:4000/)

#### Configuring log levels

UI uses [console-logging](https://www.npmjs.com/package/config-logger) for server-side logging. 
This supports log levels of `debug`, `verbose`, `info`, `warn` and `error`. By default the log level is `info` (this is configured in `config/default.json`). To set a log level of `debug`, use

```
NODE_CONFIG='{"logLevel":"debug"}' PORT=4000 FN_API_URL=http://localhost:8080 npm start

```

### Automated Testing
#### Integration tests

The Fn UI has some basic Selenium integration tests that can be used to automatically test the UI's core functionality. These tests use the `mocha` testing framework as the driver.

To run the tests:

##### 1) Install the Chrome Driver
Download the ChromeDriver from [Google](https://sites.google.com/a/chromium.org/chromedriver/downloads) and put it in a place which is in your path e.g. `/usr/local/bin/chromedriver`.

##### 2) Install the Node dev dependencies
Ensure you have the Node dev dependencies installed with:
```
npm install [--only dev]
```

##### 3) Run the Fn interface
See the instructions above for how to start the Node webserver.

##### 4) Configure your tests
Edit [test_integration/etc/config.yaml](test_integration/etc/config.yaml) accordingly e.g. point `fn_url` to your Fn UI if you're not running it at its default location.

##### 5) Run the tests
```
npm run test-integration
```