var path = require('path');
var ExtractTextPlugin = require('extract-text-webpack-plugin');
// var webpack = require('webpack');

// style
// const postcssImport = require('postcss-import');
// const postcssURL = require('postcss-url');
// const cssnext = require('postcss-cssnext');
// const fontMagician = require('postcss-font-magician');

var config = {
    entry: {
        main: './main'
    },
    output: {
        path: path.join(__dirname, './dist'),
        publicPath: '/dist/',
        filename: 'main.js'
    },
    resolve: {
        extensions: ['.js', '.vue', '.json'],
        alias: {
            'vue$': path.join(__dirname, './node_modules/vue/dist/vue.common.js'),
            'vue-router$': path.join(__dirname, './node_modules/vue-router/dist/vue-router.common.js'),
        }
    },
    module: {
        rules: [
            {
                test: /\.vue$/,
                loader: 'vue-loader',
                options: {
                    loaders: {
                        css: ExtractTextPlugin.extract({
                            use: 'css-loader',
                            fallback: 'vue-style-loader'
                        })
                    }
                }
            },
            {
                test: /\.js$/,
                loader: 'babel-loader',
                exclude: /node_modules/
            },
            {
                test: /\.css$/,
                use: ExtractTextPlugin.extract({
                    use: 'css-loader',
                    fallback: 'style-loader'
                })
            },
            {
                test: /\.(gif|jpg|png|woff|svg|eot|ttf|woff2)\??.*$/,
                loader: 'url-loader?limit=1024'
            },
        ]
    },
    plugins: [
        // new webpack.LoaderOptionsPlugin({
        //     options: {
        //         context: __dirname,
        //         postcss: [
        //             postcssImport(),
        //             fontMagician({
        //                 hosted: path.join(__dirname, './fonts/Roboto')
        //             }),
        //             postcssURL(),
        //             require('postcss-hexrgba')(),
        //             cssnext()
        //         ]
        //     }
        // }),
        new ExtractTextPlugin({
            filename: '[name].css',
            allChunks: true
        }),
    ]
};

module.exports = config;