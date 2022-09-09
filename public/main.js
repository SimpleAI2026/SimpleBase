/**
 * Adapted from jQuery Lined Textarea Plugin
 * http://alan.blog-city.com/jquerylinedtextarea.htm
 * 
 * Released under the MIT License:
 * http://www.opensource.org/licenses/mit-license.php
 */
(function ($) {
    $.fn.linedtextarea = function () {
        var fillOutLines = function (linesDiv, h, lineNo) {
            while (linesDiv.height() < h) {
                linesDiv.append("<div>" + lineNo + "</div>");
                lineNo++;
            }
            return lineNo;
        };

        return this.each(function () {
            var lineNo = 1;
            var textarea = $(this);

            /* Wrap the text area in the elements we need */
            textarea.wrap("<div class='linedtextarea' style='height:100%; overflow:hidden'></div>");
            textarea.width("calc(100% - 30px)");
            textarea.parent().prepend("<div class='lines' style='width:30px'></div>");
            var linesDiv = textarea.parent().find(".lines");

            var scroll = function (tn) {
                var domTextArea = $(this)[0];
                var scrollTop = domTextArea.scrollTop;
                var clientHeight = domTextArea.clientHeight;
                linesDiv.css({
                    'margin-top': (-scrollTop) + "px"
                });
                lineNo = fillOutLines(linesDiv, scrollTop + clientHeight,
                    lineNo);
            };
            /* React to the scroll event */
            textarea.scroll(scroll);
            $(window).resize(function () { textarea.scroll(); });
            /* We call scroll once to add the line numbers */
            textarea.scroll();
        });
    };

})(jQuery);


function ping() {
    fetch('/gossa/api/v1/ping').then(res => res.json()).then(data => {
        document.getElementById('version').innerText = data.go_version;
    });
}
ping(); // inject version info

const banner = document.getElementById('banner')
const snippet = document.getElementById('snippet')
const about = document.getElementById('about')
const funcname = document.getElementById('funcname')
const gcflags = document.getElementById('gcflags')
const horiz = document.querySelectorAll('.gutter.gutter-horizontal')
const btns = document.querySelectorAll('input[type=button]')
const code = document.getElementById('code')
const links = document.querySelectorAll('#about a')
const gutter = document.getElementsByClassName('gutter')
const output = document.getElementById('output')

function setDarkMode() {
    banner.style.backgroundColor = '#566'
    snippet.style.backgroundColor = '#665'
    about.style.backgroundColor = '#665'
    funcname.style.backgroundColor = '#343'
    funcname.style.border = '1px solid #454'
    funcname.style.color = 'lightgray'
    gcflags.style.backgroundColor = '#343'
    gcflags.style.border = '1px solid #454'
    gcflags.style.color = 'lightgray'
    horiz.forEach(v => v.style.filter = 'invert(.7)')
    btns.forEach(v => {
        v.style.backgroundColor = '#0044cb'
        v.style.color = 'lightgray'
    })
    code.style.color = 'rgb(230, 255, 255)'
    links.forEach(v => v.style.color = '#809fff')
    gutter[0].style.backgroundColor = 'rgb(45, 45, 45)'
    output.style.backgroundColor = 'rgb(60, 60, 60)'
    document.body.style.color = 'lightgray'
}

function setLightMode() {
    banner.style.backgroundColor = '#E0EBF5'
    snippet.style.backgroundColor = 'rgba(255, 252, 221, 0.81)'
    about.style.backgroundColor = '#FFD'
    funcname.style.backgroundColor = '#fff'
    funcname.style.border = '1px solid #ccc'
    funcname.style.color = ''
    gcflags.style.backgroundColor = '#fff'
    gcflags.style.border = '1px solid #ccc'
    gcflags.style.color = ''
    horiz.forEach(v => v.style.filter = '')
    btns.forEach(v => {
        v.style.backgroundColor = '#375EAB'
        v.style.color = '#fff'
    })
    code.style.color = 'black'
    links.forEach(v => v.style.color = '')
    gutter[0].style.backgroundColor = '#eee'
    output.style.backgroundColor = '#f1f1f1'
    document.body.style.color = 'black'
}

let msgbox = document.getElementById('outputMsg')
let ssabox = document.getElementById('ssa')
ssabox.addEventListener('load', () => {
    const darkBtn = ssabox.contentWindow.document.getElementById('dark-mode-button')
    if (darkBtn.checked) {
        setDarkMode()
    } else {
        setLightMode()
    }

    // inject ssa style
    $("iframe").contents().find("head").append($("<link/>", { rel: 'stylesheet', href: '/gossa/scrollbar.css', type: 'text/css' }));
    setMessageBox('', true)
    ssabox.contentWindow.document
        .getElementById('dark-mode-button')
        .addEventListener('click', function () {
            if (darkBtn.checked) {
                setDarkMode()
            } else {
                setLightMode()
            }
        })
});

window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
    ssabox.contentWindow.document.getElementById('dark-mode-button').click()
    if (e.matches) {
        setDarkMode()
    } else {
        setLightMode()
    }
    console.log('changed')
})

let lastFuncName, lastCode, lastGcflags;
function build() {
    let funcname = document.getElementById('funcname').value;
    let code = document.getElementById('code').value;
    let gcflags = document.getElementById('gcflags').value;

    // several early checks
    if (funcname === lastFuncName && code === lastCode && gcflags === lastGcflags) {
        console.log('no changes, do not submit')
        return
    }
    if (!findSSAFunc(code, funcname)) {
        setMessageBox('GOSSAFUNC does not exist in your code.', false)
        return
    }

    lastFuncName = funcname
    lastCode = code
    lastGcflags = gcflags
    setMessageBox('Waiting for response...', false)

    fetch('/gossa/api/v1/buildssa', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            'funcname': funcname,
            'code': code,
            'gcflags': gcflags,
        }),
    })
        .then((response) => {
            return new Promise((resolve, reject) => {
                // will resolve or reject depending on status, will pass both "status" and "data" in either case
                let func;
                response.status < 400 ? func = resolve : func = reject;
                response.json().then(data => func({ 'status': response.status, 'data': data }));
            });
        })
        .then(res => {
            ssabox.src = `/gossa/buildbox/${res.data.build_id}/ssa.html`

            // update url
            const param = 'id=' + res.data.build_id
            history.pushState(null, null, document.location.href.split('?')[0] + '?' + param)
        })
        .catch(res => setMessageBox(res.data.msg, false));
}

const methodPattern = /^\([\w\*]+\)\.\w+$/
const globPattern = /^glob\.\.func\d+(\.\d)*$/
const anonyPattern = /^\w+\.func\d+(\.\d)*$/

function findSSAFunc(code, funcname) {
    if (funcname.indexOf('.') != -1) {
        if (funcname[0] == '(') {
            return methodPattern.test(funcname)
        } else if (funcname.startsWith("glob")) {
            return globPattern.test(funcname)
        } else {
            return anonyPattern.test(funcname)
        }
    }

    let funcDclPattern = new RegExp(`func[ \\t]+${funcname}[ \\t]*\\(`)
    return funcDclPattern.test(code)
}

function setMessageBox(msg, hide) {
    msgbox.innerText = msg
    if (hide) {
        ssabox.style.display = '';
        msgbox.style.display = 'none';
        return
    }
    ssabox.style.display = 'none';
    msgbox.style.display = '';
}

// listen build action
let buildssa = document.getElementById('build')
buildssa.addEventListener('click', () => {
    build()
})

// textarea and tab key stroke
$('#code').linedtextarea();
$('#code').keydown(function (event) {
    if (event.keyCode == 9) {
        event.preventDefault();
        let start = this.selectionStart;
        let end = this.selectionEnd;
        // set textarea value to: text before caret + tab + text after caret
        $(this).val($(this).val().substring(0, start)
            + "\t"
            + $(this).val().substring(end));
        // put caret at right position again
        this.selectionStart =
            this.selectionEnd = start + 1;
    }
});

function loadCode() {
    const params = new URLSearchParams(window.location.search);
    const id = params.get('id');
    if (id === null || id === undefined || id === '') { return; }
    ssabox.src = `/gossa/buildbox/${id}/ssa.html`
    // load code
    fetch(`/gossa/buildbox/${id}/main.go`)
        .then((response) => {
            return new Promise((resolve, reject) => {
                let func; response.status < 400 ? func = resolve : func = reject;
                response.text().then(data => func({ 'status': response.status, 'data': data }));
            });
        })
        .then(res => {
            console.log(res)
            document.getElementById('code').textContent = res.data
        })
        .catch(res => {
            fetch(`/gossa/buildbox/${id}/main_test.go`)
                .then(res => res.text())
                .then(res => {
                    console.log(res)
                    document.getElementById('code').textContent = res
                })
            // if still fail? don't handle anything
        });
}

loadCode() // load content if access with id

Split(['#snippet', '#output'], { sizes: [30, 70] })