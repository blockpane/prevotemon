function toggle() {
    document.getElementById('pauseSwitch').click()
}

function unselect() {
    if (document.getElementById('pauseSwitch').checked === false) {
        window.location.hash = ""
        document.getElementById("forward").hidden = true
        document.getElementById("paused").hidden = true
        document.getElementById("playing").hidden = false
        document.getElementById("playing").innerHTML = `
        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" fill="currentColor" class="bi bi-pause" viewBox="0 0 16 16">
              <path d="M6 3.5a.5.5 0 0 1 .5.5v8a.5.5 0 0 1-1 0V4a.5.5 0 0 1 .5-.5zm4 0a.5.5 0 0 1 .5.5v8a.5.5 0 0 1-1 0V4a.5.5 0 0 1 .5-.5z"/>
            </svg>`
    } else {
        document.getElementById("forward").hidden = false
        document.getElementById("playing").hidden = true
        document.getElementById("paused").hidden = false
        document.getElementById("paused").innerHTML = `
        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" fill="currentColor" class="bi bi-play" viewBox="0 0 16 16">
            <path d="M10.804 8 5 4.633v6.734L10.804 8zm.792-.696a.802.802 0 0 1 0 1.392l-6.363 3.692C4.713 12.69 4 12.345 4 11.692V4.308c0-.653.713-.998 1.233-.696l6.363 3.692z"/>
            </svg>`
    }
}

function navBlock(b = -1) {
    const current = document.getElementById("blocknum").innerText
    let n = parseInt(current)
    if (isNaN(n)) {
        n = parseInt(window.location.hash.replace("#", ""))
        if (isNaN(n)) {
            return
        }
    }
    window.location.hash = n+b
    if (document.getElementById('pauseSwitch').checked === false) {
        document.getElementById('pauseSwitch').click()
    }
    document.getElementById("forward").hidden = false
}

let skipUpdate = false

async function chartPrevotes() {


    let initialVotes = []
    let initialState = {}

    let height = 0
    let waitForRound = false
    let busy = false

    async function getState() {
        if (busy) {
            return
        }
        busy = true
        const hash = window.location.hash
        const h = parseInt(hash.replace("#", ""), 10)
        console.log(h)
        let endpoint = "/state"
        if (!isNaN(h) && h >= 1) {
            endpoint = `/history?height=${h}`
            if (document.getElementById('pauseSwitch').checked === false) {
                document.getElementById('pauseSwitch').click()
            }
            skipUpdate = true
        }
        const response = await fetch(endpoint, {
            method: 'GET',
            mode: 'cors',
            cache: 'no-cache',
            credentials: 'same-origin',
            redirect: 'error',
            referrerPolicy: 'no-referrer'
        });
        try {
            initialState = await response.json()
        } catch {
            if (!isNaN(h)) {
                document.location.hash = ""
                busy = false
                return
            }
        }

        for (const v of initialState.pre_votes) {
            if (v.offset_ms < -1000) {
                continue
            }
            let size = v.weight * 15 ^ 2
            if (size < 15) {
                size = 15
            }
            initialVotes.push([v.offset_ms, v.weight, size, v.moniker, "votes"])
        }
        if (initialState.round == null) {
            document.location.hash = ""
            busy = false
            return
        }
        height = initialState.round.height
        if (!isNaN(h)) {
            window.location.hash = height
        }
        document.getElementById('blocknum').innerText = initialState.round.height
        document.getElementById('proposer').innerText = initialState.round.proposer
        waitForRound = true
        busy = false
    }
    await getState()

    let pctChartDom = document.getElementById('percent');
    let pctChart = echarts.init(pctChartDom);
    let pctOption;

    pctOption = {
        series: [
            {
                type: 'gauge',
                animationDurationUpdate: 150,
                progress: {
                    show: true,
                    width: 12,
                    itemStyle: {
                        color: {
                            type: 'linear',
                            x: 0,
                            y: 0,
                            x2: 0,
                            y2: 1,
                            colorStops: [{
                                offset: 0, color: 'rgb(69,51,120)'
                            }, {
                                offset: 1, color: 'rgba(89,71,190,0.5)'
                            }],
                            global: false
                        }
                    }
                },
                pointer: {
                    itemStyle: {
                        color: 'rgb(89,71,190)'
                    }
                },

                axisLine: {
                    lineStyle: {
                        width: 12,
                    }
                },
                axisTick: {
                    show: false
                },
                splitLine: {
                    length: 15,
                    lineStyle: {
                        width: 2,
                        color: 'rgb(79,61,180)'
                    }
                },
                axisLabel: {
                    distance: 25,
                    color: 'rgba(136,220,3,0.4)',
                    fontSize: 8
                },
                anchor: {
                    show: true,
                    showAbove: true,
                    size: 25,
                    itemStyle: {
                        borderWidth: 10,
                        color: 'rgb(89,71,190)'
                    }
                },
                title: {
                    show: false
                },
                detail: {
                    valueAnimation: true,
                    fontSize: 32,
                    offsetCenter: [0, '70%'],
                    color: "white",
                },
                data: [ 0 ],
            }
        ]
    };

    pctOption && pctChart.setOption(pctOption);


    let chartDom = document.getElementById('votes');
    let myChart = echarts.init(chartDom);
    let option

    let dedup = {}
    option = {
        backgroundColor: "transparent",
        title: {
            text: 'Prevotes by Time and Consensus Power',
            left: '5%',
            top: '3%'
        },
        grid: {
            left: '8%',
            top: '10%'
        },
        xAxis: {
            splitLine: {
                lineStyle: {
                    type: 'dotted',
                    color: "grey"
                }
            },
           scale: true,
            name: "Milliseconds",
        },
        yAxis: {
            splitLine: {
                show: false,
            },
            scale: true,
            type: "log",
            logBase: 2,
            name: '% Consensus Power'
        },
        series: [
            {
                name: 'votes',
                data: initialVotes,
                type: 'scatter',
                symbol: "circle",
                symbolSize: function (data) {
                    return data[2]
                },
                label: {
                    show: true,
                    formatter: function (param) {
                        return param.data[3].substring(0, 14);
                    },
                    fontSize: 9,
                    fontWeight: "lighter",
                },
                emphasis: {
                    focus: 'series',
                    label: {
                        show: true,
                        formatter: function (param) {
                            return `${param.data[3]}: ${param.data[1]}% ${param.data[0]/1000.0} seconds`;
                        },
                        position: 'top',
                        color: "white",
                        backgroundColor: 'rgba(0,0,0,0.6)',
                        fontSize: 24,
                    }
                },
                itemStyle: {
                    shadowBlur: 10,
                    shadowColor: 'rgba(255,159,0,0.2)',
                    shadowOffsetY: 1,
                    color: new echarts.graphic.RadialGradient(0.8, 0.8, 1, [
                        {
                            offset: 0,
                            //color: 'rgb(255,166,84)'
                            color: 'rgb(107,59,177)',
                        },
                        {
                            offset: 1,
                            //color: 'rgb(101,9,21)'
                            color: 'rgb(19,14,31)',
                        }
                    ])
                },
            }
        ]
    };

    option && myChart.setOption(option);

    let lastLogBase = "0"
    let lastHash = ""
    setInterval(pause, 100);
    async function pause() {
        const base = document.getElementById('timeScale').value
        const hash = window.location.hash
        if (base !== lastLogBase) {
            switch (base) {
                case "0":
                    option.xAxis.type = 'value'
                    break
                case "2":
                    option.xAxis.type = 'log'
                    option.xAxis.logBase = 2
                    break
                case "10":
                    option.xAxis.type = 'log'
                    option.xAxis.logBase = 10
                    break
                case "32":
                    option.xAxis.type = 'log'
                    option.xAxis.logBase = 32
            }
            myChart.setOption(option)
            lastLogBase = base
        }
        if (document.getElementById('pauseSwitch').checked === true && lastHash === hash) {
            skipUpdate = true
        } else if (skipUpdate === true || lastHash !== hash) {
            document.getElementById('timeScale').value = "0"
            option.xAxis.type = 'value'
            initialVotes = []
            await getState()
            option.series[0].data = initialVotes
            myChart.setOption(option)
            document.getElementById('blocknum').innerText = initialState.round.height
            document.getElementById('proposer').innerText = initialState.round.proposer
            pctOption.series[0].data = [ initialState.progress.pct ]
            pctChart.setOption(pctOption)
            if (lastHash === hash) {
                skipUpdate = false
            }
        }
        lastHash = hash
    }

    let wsProto = "ws://"
    if (location.protocol === "https:") {
        wsProto = "wss://"
    }

    let currentRound = 0
    function connectRounds() {
        const socket = new WebSocket(wsProto + location.host + '/rounds/ws');
        socket.addEventListener('message', function (event) {
            const updVote = JSON.parse(event.data);
            if (skipUpdate === true) {
                return
            }
            if (updVote.type === "round"){
                waitForRound = false
                currentRound = updVote.height
                initialVotes = []
                dedup = {}
                option.series[0].data = initialVotes
                myChart.setOption(option)
                document.getElementById('blocknum').innerText = updVote.height
                document.getElementById('proposer').innerText = updVote.proposer
            } else if (updVote.type === "new_proposer") {
                document.getElementById('proposer').innerText = updVote.proposer
            //} else if (updVote.type === "final" && updVote.height >= currentRound) {
            } else if (updVote.type === "final") {
                waitForRound = false
                initialVotes = []
                for (const v of updVote.Votes) {
                    if (v.offset_ms < -1000) {
                        continue
                    }
                    let size = v.weight * 15 ^ 2
                    if (size < 15) {
                        size = 15
                    }
                    initialVotes.push([v.offset_ms, v.weight, size, v.moniker, "votes"])
                }
                option.series[0].data = initialVotes
                myChart.setOption(option)
                document.getElementById('blocknum').innerText = updVote.height
                document.getElementById('proposer').innerText = updVote.proposer
                pctOption.series[0].data = [ updVote.percent ]
                pctChart.setOption(pctOption)
                if (document.getElementById('pauseSwitch').checked === true) {
                    skipUpdate = true
                }
            }
        });
        socket.onclose = function(e) {
            console.log('Socket is closed, retrying /prevote/ws ...', e.reason);
            setTimeout(function() {
                connectRounds();
            }, 4000);
        };
    }
    connectRounds()

    function connectProgress() {
        let lastPct = 0.0
        const socket = new WebSocket(wsProto + location.host + '/progress/ws');
        socket.addEventListener('message', function (event) {
            const updPct = JSON.parse(event.data);
            if (updPct.type === "pct" && updPct.pct !== lastPct && skipUpdate === false) {
                lastPct = updPct.pct
                pctOption.series[0].data = [ updPct.pct ]
                pctChart.setOption(pctOption)
            }
        });
        socket.onclose = function(e) {
            console.log('Socket is closed, retrying /progress/ws ...', e.reason);
            setTimeout(function() {
                connectProgress();
            }, 4000);
        };
    }
    connectProgress()

    let lastSize = 0
    let interval = 75
    const userAgent = navigator.userAgent
    if(userAgent.match(/firefox|fxios/i)){
        interval = 250
    }
    setInterval(update, interval);
    function update() {
        if (lastSize !== initialVotes.length && skipUpdate === false) {
            lastSize = initialVotes.length
            option.series[0].data = initialVotes
            myChart.setOption(option)
        }
    }

    function connectVotes() {
        const socket = new WebSocket(wsProto + location.host + '/prevote/ws');
        socket.addEventListener('message', function (event) {
            const updVote = JSON.parse(event.data);
            if (updVote.type === "prevote" && dedup[updVote.valoper] !== true && skipUpdate === false && waitForRound === false) {
                if (updVote.height < height) {
                    return
                }
                if (updVote.offset_ms < -1000) {
                    console.log(`invalid offset for ${updVote.moniker}: ${updVote.offset_ms}`)
                } else {
                    dedup[updVote.valoper] = true
                    let size = updVote.weight * 15 ^ 2
                    if (size < 15) {
                        size = 15
                    }
                    if (updVote.height > height) {
                        height = updVote.height
                        initialVotes = []
                        initialVotes.push([updVote.offset_ms, updVote.weight, size, updVote.moniker, "votes"])
                        option.series[0].data = initialVotes
                        myChart.setOption(option)
                    } else {
                        initialVotes.push([updVote.offset_ms, updVote.weight, size, updVote.moniker, "votes"])
                    }
                }
            }
        });
        socket.onclose = function(e) {
            console.log('Socket is closed, retrying /prevote/ws ...', e.reason);
            setTimeout(function() {
                connectVotes();
            }, 4000);
        };
    }
    connectVotes()
}