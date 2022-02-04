async function chartPrevotes() {

    const response = await fetch("/state", {
        method: 'GET',
        mode: 'cors',
        cache: 'no-cache',
        credentials: 'same-origin',
        redirect: 'error',
        referrerPolicy: 'no-referrer'
    });
    let initialState = await response.json()

    let initialVotes = []
    for (const v of initialState.pre_votes) {
        let size = v.weight*15^2
        if (size < 15) {
            size = 15
        }
        initialVotes.push([v.offset_ms, v.weight, size, v.moniker, "votes"])
    }
    console.log(initialVotes)
    document.getElementById('blocknum').innerText = initialState.round.height
    document.getElementById('proposer').innerText = initialState.round.proposer

    let pctChartDom = document.getElementById('percent');
    let pctChart = echarts.init(pctChartDom);
    let pctOption;

    pctOption = {
        series: [
            {
                type: 'gauge',
                progress: {
                    show: true,
                    width: 12,
                    itemStyle: {
                        //color: ['rgb(89,71,190)', 'rgb(136,220,3)',]
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
                            global: false // default is false
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
                        //color: '#999'
                        color: 'rgb(79,61,180)'
                    }
                },
                axisLabel: {
                    distance: 25,
                    color: 'rgba(136,220,3,0.4)',
                    //color: '#999',
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
    let option;

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
            }
        },
        yAxis: {
            splitLine: {
                show: false,
            },
            scale: true,
            type: "log",
            logBase: 2,
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

    let wsProto = "ws://"
    if (location.protocol === "https:") {
        wsProto = "wss://"
    }

    function connectRounds() {
        const socket = new WebSocket(wsProto + location.host + '/rounds/ws');
        socket.addEventListener('message', function (event) {
            const updVote = JSON.parse(event.data);
            if (updVote.type === "round") {
                //console.log(updVote)
                initialVotes = []
                dedup = {}
                myChart.setOption(option)
                document.getElementById('blocknum').innerText = updVote.height
                document.getElementById('proposer').innerText = updVote.proposer
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
            if (updPct.type === "pct" && updPct.pct !== lastPct) {
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
    setInterval(update, 100);
    function update() {
        if (lastSize !== initialVotes.length) {
            lastSize = initialVotes.length
            option.series[0].data = initialVotes
            myChart.setOption(option)
        }
    }

    function connectVotes() {
        const socket = new WebSocket(wsProto + location.host + '/prevote/ws');
        socket.addEventListener('message', function (event) {
            const updVote = JSON.parse(event.data);
            if (updVote.type === "prevote" && dedup[updVote.valoper] !== true) {
                dedup[updVote.valoper] = true
                let size = updVote.weight*15^2
                if (size < 15) {
                    size = 15
                }
                initialVotes.push([updVote.offset_ms, updVote.weight, size, updVote.moniker, "votes"])
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